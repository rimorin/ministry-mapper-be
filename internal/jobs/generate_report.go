package jobs

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"sync"
	"text/template"
	"time"

	"github.com/mailersend/mailersend-go"
	"github.com/pocketbase/pocketbase/core"
)

// reportTmpl is parsed once on first use and reused for all email sends.
var (
	reportTmpl     *template.Template
	reportTmplOnce sync.Once
	reportTmplErr  error
)

func getReportTemplate() (*template.Template, error) {
	reportTmplOnce.Do(func() {
		reportTmpl, reportTmplErr = template.ParseFiles("templates/report.html")
	})
	return reportTmpl, reportTmplErr
}

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
	CongregationName string
	CongregationCode string
	ReportDate       string
	ReportTitle      string
	FileName         string
	RecipientName    string
	IsOnDemand       bool
	Summary          SummaryData
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

	if err := sendReportEmailToRecipient(app, congregation, filename, content, recipient, generateAISummary(app, congregation, IsAISummaryEnabled(), period), period); err != nil {
		return fmt.Errorf("failed to send email for congregation %s: %v", congregation.Get("code"), err)
	}

	return nil
}

// generateAISummary calls the OpenAI API to produce a narrative for the given period.
// Returns SummaryData{} with Available=false on any failure so the template gracefully omits the section.
func generateAISummary(app core.App, congregation *core.Record, aiEnabled bool, period ReportPeriod) SummaryData {
	if !aiEnabled {
		return SummaryData{}
	}

	client := newLLMClient()
	if client == nil {
		log.Printf("AI summary skipped for %s: OPENAI_API_KEY not set", congregation.Get("code"))
		return SummaryData{}
	}

	data, err := BuildSummaryData(app, congregation, period)
	if err != nil {
		log.Printf("AI summary: failed to build data for %s: %v", congregation.Get("code"), err)
		return SummaryData{}
	}

	systemMsg, userMsg := BuildPrompt(data)

	llmResp, err := client.generateSummary(systemMsg, userMsg)
	if err != nil {
		log.Printf("AI summary: LLM call failed for %s: %v", congregation.Get("code"), err)
		return SummaryData{}
	}

	data.Available = true
	data.Coverage = llmResp.Coverage
	data.NeedsAttention = llmResp.NeedsAttention
	return data
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

	tmpl, err := getReportTemplate()
	if err != nil {
		log.Println("Error parsing template:", err)
		return err
	}

	congregationName, _ := congregation.Get("name").(string)
	congregationCode, _ := congregation.Get("code").(string)
	emailData := ReportTemplateData{
		CongregationName: congregationName,
		CongregationCode: congregationCode,
		ReportDate:       period.Label,
		ReportTitle:      "Monthly Report",
		FileName:         filename,
		IsOnDemand:       false,
		Summary:          generateAISummary(app, congregation, aiEnabled, period),
	}

	var body bytes.Buffer
	if err := tmpl.Execute(&body, emailData); err != nil {
		log.Println("Error executing template:", err)
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
	message.SetSubject(fmt.Sprintf("Monthly Report for %s - %s", congregation.Get("name"), period.Label))
	message.SetHTML(body.String())
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

	tmpl, err := getReportTemplate()
	if err != nil {
		log.Println("Error parsing template:", err)
		return err
	}

	congregationName, _ := congregation.Get("name").(string)
	congregationCode, _ := congregation.Get("code").(string)
	emailData := ReportTemplateData{
		CongregationName: congregationName,
		CongregationCode: congregationCode,
		ReportDate:       period.Label,
		ReportTitle:      "Activity Report",
		FileName:         filename,
		RecipientName:    name,
		IsOnDemand:       true,
		Summary:          summary,
	}

	var body bytes.Buffer
	if err := tmpl.Execute(&body, emailData); err != nil {
		log.Println("Error executing template:", err)
		return err
	}

	encoded := base64.StdEncoding.EncodeToString(content)
	ms := mailersend.NewMailersend(os.Getenv("MAILERSEND_API_KEY"))

	message := ms.Email.NewMessage()
	message.SetFrom(mailersend.From{Email: os.Getenv("MAILERSEND_FROM_EMAIL"), Name: "Ministry Mapper"})
	message.SetRecipients([]mailersend.Recipient{{Email: email, Name: name}})
	message.SetSubject(fmt.Sprintf("Activity Report for %s - %s", congregationName, period.Label))
	message.SetHTML(body.String())
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
