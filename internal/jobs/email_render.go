package jobs

import (
	"bytes"
	"fmt"
	"html/template"
	"regexp"
	"strings"
	"sync"

	xhtml "golang.org/x/net/html"
)

// templateDir is where the email templates live, relative to the working
// directory. The binary runs from the repo root; tests point it at ../../templates.
var templateDir = "templates"

// emailChrome is the part of every email the shared layout renders: the inbox
// preview line, the slim header, the title block, the optional button and the
// footer. Each email's data struct embeds it.
type emailChrome struct {
	Preheader   string
	Kicker      string
	Title       string
	Subtitle    string
	ButtonLabel string
	ButtonURL   string
	Footer      string
}

var emailTemplates sync.Map // content file name -> *template.Template

// renderEmail renders a content template inside layout.html and returns the HTML
// body plus a plain-text alternative derived from it.
func renderEmail(content string, data any) (html, text string, err error) {
	tmpl, err := emailTemplate(content)
	if err != nil {
		return "", "", err
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "layout", data); err != nil {
		return "", "", fmt.Errorf("render %s: %w", content, err)
	}
	return buf.String(), htmlToText(buf.String()), nil
}

func emailTemplate(content string) (*template.Template, error) {
	if cached, ok := emailTemplates.Load(content); ok {
		return cached.(*template.Template), nil
	}
	tmpl, err := template.ParseFiles(templateDir+"/layout.html", templateDir+"/"+content)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", content, err)
	}
	emailTemplates.Store(content, tmpl)
	return tmpl, nil
}

var (
	voidElements  = map[string]bool{"br": true, "img": true, "meta": true, "link": true, "hr": true, "input": true}
	blockElements = map[string]bool{"p": true, "div": true, "tr": true, "li": true, "h1": true, "h2": true, "h3": true, "table": true}
	blankLines    = regexp.MustCompile(`\n{3,}`)
	spaces        = regexp.MustCompile(`[ \t\r\n]+`)
)

// htmlToText derives the plain-text part of an email from its HTML: block
// elements become line breaks, table cells are spaced apart, links keep their
// destination, and styles, the head and the hidden preheader are dropped.
func htmlToText(src string) string {
	z := xhtml.NewTokenizer(strings.NewReader(src))
	var out strings.Builder
	skipDepth := 0
	href := ""
	linkText := ""
	for {
		switch z.Next() {
		case xhtml.ErrorToken:
			return blankLines.ReplaceAllString(strings.TrimSpace(trimLines(out.String())), "\n\n")
		case xhtml.StartTagToken, xhtml.SelfClosingTagToken:
			t := z.Token()
			selfClosing := t.Type == xhtml.SelfClosingTagToken || voidElements[t.Data]
			if skipDepth > 0 {
				if !selfClosing {
					skipDepth++
				}
				continue
			}
			switch {
			case t.Data == "head" || t.Data == "style" || t.Data == "script" || t.Data == "title":
				skipDepth++
			case t.Data == "div" && strings.Contains(attr(t, "style"), "display:none"):
				skipDepth++
			case t.Data == "br":
				out.WriteString("\n")
			case blockElements[t.Data]:
				out.WriteString("\n")
			case t.Data == "a":
				href, linkText = attr(t, "href"), ""
			}
		case xhtml.EndTagToken:
			t := z.Token()
			if skipDepth > 0 {
				skipDepth--
				continue
			}
			switch {
			case blockElements[t.Data]:
				out.WriteString("\n")
			case t.Data == "td":
				out.WriteString("  ")
			case t.Data == "a":
				if href != "" && href != "#" && !strings.Contains(linkText, href) {
					out.WriteString(" (" + href + ")")
				}
				href = ""
			}
		case xhtml.TextToken:
			if skipDepth == 0 {
				text := spaces.ReplaceAllString(string(z.Text()), " ")
				out.WriteString(text)
				if href != "" {
					linkText += text
				}
			}
		}
	}
}

func attr(t xhtml.Token, name string) string {
	for _, a := range t.Attr {
		if a.Key == name {
			return a.Val
		}
	}
	return ""
}

func trimLines(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(l)
	}
	return strings.Join(lines, "\n")
}
