package jobs

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/mailersend/mailersend-go"
	"github.com/pocketbase/pocketbase/core"
)

// sendEmailWithRetry retries a MailerSend send call up to 3 times with exponential backoff.
func sendEmailWithRetry(send func() error) error {
	delays := []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second}
	var err error
	for attempt := 0; attempt <= len(delays); attempt++ {
		err = send()
		if err == nil {
			return nil
		}
		if attempt < len(delays) {
			log.Printf("Email send attempt %d failed: %v — retrying in %s", attempt+1, err, delays[attempt])
			time.Sleep(delays[attempt])
		}
	}
	return fmt.Errorf("email send failed after %d attempts: %w", len(delays)+1, err)
}

type ReportTemplateData struct {
	emailChrome
	ReportDate string
	FileName   string
	Summary    SummaryData
}

func GenerateMonthlyReport(app core.App, aiEnabled bool) {
	log.Println("Starting monthly report generation...")

	congregations, err := app.FindRecordsByFilter("congregations", "", "", 0, 0)
	if err != nil {
		log.Printf("Failed to fetch congregations: %v", err)
		return
	}

	for _, congregation := range congregations {
		if err := GenerateAndSendCongregationReport(app, congregation, aiEnabled); err != nil {
			log.Printf("Failed to generate and send report for congregation %s: %v", congregation.Get("code"), err)
			continue
		}
	}

	log.Println("Monthly report generation completed.")
}

// generateReportBuffer builds the Excel report for a congregation and returns
// the filename and raw bytes. Shared by both send variants below.
// GenerateAndSendCongregationReport generates the Excel report and emails it
// to all administrators of the congregation. Used by the monthly scheduled job.
func GenerateAndSendCongregationReport(app core.App, congregation *core.Record, aiEnabled bool) error {
	period := PreviousCalendarMonth()
	filename, content, err := generateReportBuffer(app, congregation, period)
	if err != nil {
		return err
	}

	if err := sendReportEmailFromBuffer(app, congregation, filename, content, aiEnabled, period); err != nil {
		return fmt.Errorf("failed to send email for congregation %s: %v", congregation.Get("code"), err)
	}

	return nil
}

// GenerateAndSendCongregationReportToUser generates the Excel report and emails it
// only to the specified recipient. Used by the on-demand report endpoint.
func GenerateAndSendCongregationReportToUser(app core.App, congregation *core.Record, recipient *core.Record) error {
	period := RollingDays(OnDemandReportDays)
	filename, content, err := generateReportBuffer(app, congregation, period)
	if err != nil {
		return err
	}

	if err := sendReportEmailToRecipient(app, congregation, filename, content, recipient, buildEmailSummary(app, congregation, IsAISummaryEnabled(), period), period); err != nil {
		return fmt.Errorf("failed to send email for congregation %s: %v", congregation.Get("code"), err)
	}

	return nil
}

// buildEmailSummary assembles the figures the email shows and, when the AI
// summary is enabled, asks the model for the two narrative paragraphs. The
// figures never depend on the model: Available only gates the narrative, and a
// narrative that quotes a number or name not in the prompt is dropped.
func buildEmailSummary(app core.App, congregation *core.Record, aiEnabled bool, period ReportPeriod) SummaryData {
	data, err := BuildSummaryData(app, congregation, period)
	if err != nil {
		log.Printf("Report summary: failed to build data for %s: %v", congregation.Get("code"), err)
		return SummaryData{}
	}
	if !aiEnabled {
		return data
	}

	client := newLLMClient()
	if client == nil {
		log.Printf("AI summary skipped for %s: OPENAI_API_KEY not set", congregation.Get("code"))
		return data
	}

	systemMsg, userMsg := BuildPrompt(data)
	llmResp, err := client.generateSummary(systemMsg, userMsg)
	if err != nil {
		log.Printf("AI summary: LLM call failed for %s: %v", congregation.Get("code"), err)
		return data
	}
	if err := checkNarrative(llmResp, userMsg); err != nil {
		log.Printf("AI summary dropped for %s: %v", congregation.Get("code"), err)
		return data
	}

	data.Available = true
	data.Coverage = llmResp.Coverage
	data.NeedsAttention = llmResp.NeedsAttention
	return data
}

// reportChrome fills the shared email frame for a report: the inbox preview
// line, the title block, the app button and the footer.
func reportChrome(congregationName string, period ReportPeriod, summary SummaryData) emailChrome {
	kicker := "Monthly report"
	if period.IsOnDemand {
		kicker = "Activity report"
	}
	preheader := "Activity report for " + period.Label + "."
	if summary.Visits > 0 {
		preheader = fmt.Sprintf("%d homes reached, %s.", summary.HouseholdsReached, todoPhrase(len(summary.ActionItems())))
	}
	return emailChrome{
		Preheader:   preheader,
		Kicker:      kicker,
		Title:       congregationName,
		Subtitle:    period.Label,
		ButtonLabel: "Open Ministry Mapper",
		ButtonURL:   os.Getenv("PB_APP_URL"),
		Footer:      fmt.Sprintf("Sent to administrators of %s. Reply to this email if a figure looks wrong.", congregationName),
	}
}

// reportSubject leads with the two numbers an administrator wants when the
// period had visits, and falls back to the report kind and period otherwise.
func reportSubject(congregationName string, period ReportPeriod, summary SummaryData) string {
	if summary.Visits > 0 {
		return fmt.Sprintf("%s: %d homes reached, %s", congregationName, summary.HouseholdsReached, todoPhrase(len(summary.ActionItems())))
	}
	kind := "Monthly report"
	if period.IsOnDemand {
		kind = "Activity report"
	}
	return fmt.Sprintf("%s for %s, %s", kind, congregationName, period.Label)
}

func todoPhrase(n int) string {
	switch n {
	case 0:
		return "nothing to do"
	case 1:
		return "1 thing to do"
	}
	return fmt.Sprintf("%d things to do", n)
}

func sendReportEmailFromBuffer(app core.App, congregation *core.Record, filename string, content []byte, aiEnabled bool, period ReportPeriod) error {
	log.Printf("Sending report email for congregation: %s", congregation.Get("code"))

	if os.Getenv("MAILERSEND_API_KEY") == "" {
		return fmt.Errorf("MAILERSEND_API_KEY is not configured")
	}
	if os.Getenv("MAILERSEND_FROM_EMAIL") == "" {
		return fmt.Errorf("MAILERSEND_FROM_EMAIL is not configured")
	}

	recipients, err := fetchCongregationRecipients(app, congregation.Id, true)
	if err != nil {
		log.Println("Error fetching recipients:", err)
		return err
	}

	if len(recipients) == 0 {
		return fmt.Errorf("no administrator recipients found for congregation %s — report not sent", congregation.Get("code"))
	}

	log.Printf("Processing %d recipients\n", len(recipients))

	congregationName, _ := congregation.Get("name").(string)
	summary := buildEmailSummary(app, congregation, aiEnabled, period)
	emailData := ReportTemplateData{
		emailChrome: reportChrome(congregationName, period, summary),
		ReportDate:  period.Label,
		FileName:    filename,
		Summary:     summary,
	}
	htmlBody, textBody, err := renderEmail("report.html", emailData)
	if err != nil {
		log.Println("Error rendering report email:", err)
		return err
	}

	encoded := base64.StdEncoding.EncodeToString(content)
	ms := mailersend.NewMailersend(os.Getenv("MAILERSEND_API_KEY"))

	emailRecipients := []mailersend.Recipient{}
	for _, r := range recipients {
		emailRecipients = append(emailRecipients, mailersend.Recipient{
			Email: r.Email,
			Name:  r.Name,
		})
	}

	message := ms.Email.NewMessage()
	message.SetFrom(mailersend.From{Email: os.Getenv("MAILERSEND_FROM_EMAIL"), Name: "Ministry Mapper"})
	message.SetRecipients(emailRecipients)
	message.SetSubject(reportSubject(congregationName, period, summary))
	message.SetHTML(htmlBody)
	message.SetText(textBody)
	message.AddAttachment(mailersend.Attachment{Filename: filename, Content: encoded})

	if err := sendEmailWithRetry(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_, err := ms.Email.Send(ctx, message)
		return err
	}); err != nil {
		log.Printf("Error sending report email for %s: %v", congregation.Get("code"), err)
		return err
	}

	log.Println("Report email sent successfully")
	return nil
}

// sendReportEmailToRecipient sends the report to a single specified recipient
// instead of all congregation administrators. Used for on-demand report requests.
func sendReportEmailToRecipient(app core.App, congregation *core.Record, filename string, content []byte, recipient *core.Record, summary SummaryData, period ReportPeriod) error {
	name, _ := recipient.Get("name").(string)
	email, _ := recipient.Get("email").(string)
	if email == "" {
		return fmt.Errorf("recipient has no email address")
	}

	if os.Getenv("MAILERSEND_API_KEY") == "" {
		return fmt.Errorf("MAILERSEND_API_KEY is not configured")
	}
	if os.Getenv("MAILERSEND_FROM_EMAIL") == "" {
		return fmt.Errorf("MAILERSEND_FROM_EMAIL is not configured")
	}

	log.Printf("Sending on-demand report for congregation %s to %s", congregation.Get("code"), email)

	congregationName, _ := congregation.Get("name").(string)
	emailData := ReportTemplateData{
		emailChrome: reportChrome(congregationName, period, summary),
		ReportDate:  period.Label,
		FileName:    filename,
		Summary:     summary,
	}
	htmlBody, textBody, err := renderEmail("report.html", emailData)
	if err != nil {
		log.Println("Error executing template:", err)
		return err
	}

	encoded := base64.StdEncoding.EncodeToString(content)
	ms := mailersend.NewMailersend(os.Getenv("MAILERSEND_API_KEY"))

	message := ms.Email.NewMessage()
	message.SetFrom(mailersend.From{Email: os.Getenv("MAILERSEND_FROM_EMAIL"), Name: "Ministry Mapper"})
	message.SetRecipients([]mailersend.Recipient{{Email: email, Name: name}})
	message.SetSubject(reportSubject(congregationName, period, summary))
	message.SetHTML(htmlBody)
	message.SetText(textBody)
	message.AddAttachment(mailersend.Attachment{Filename: filename, Content: encoded})

	if err := sendEmailWithRetry(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_, err := ms.Email.Send(ctx, message)
		return err
	}); err != nil {
		log.Printf("Error sending on-demand report email: %v", err)
		return err
	}

	log.Println("On-demand report email sent successfully")
	return nil
}
