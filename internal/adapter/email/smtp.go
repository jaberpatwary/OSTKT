package email

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"strings"
	"time"

	mail "gopkg.in/mail.v2"

	"ticket-system/internal/config"
	"ticket-system/internal/domain"
)

type Sender struct{ config config.Config }

func NewSender(cfg config.Config) *Sender { return &Sender{config: cfg} }

func (s *Sender) Enabled() bool {
	return s.config.SMTPHost != "" && s.config.SMTPUser != "" && s.config.SMTPPassword != "" && s.config.SMTPFrom != ""
}

func (s *Sender) SendTicket(ctx context.Context, ticket domain.Ticket, location *time.Location) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	htmlBody, textBody, err := renderTicketEmail(ticket, location)
	if err != nil {
		return err
	}
	message := mail.NewMessage()
	message.SetAddressHeader("From", s.config.SMTPFrom, s.config.SMTPFromName)
	message.SetAddressHeader("To", ticket.Email, ticket.EmployeeName)
	message.SetHeader("Subject", fmt.Sprintf("✅ আপনার টিকিট সম্পন্ন হয়েছে — %s", ticket.TicketNumber))
	message.SetBody("text/plain", textBody)
	message.AddAlternative("text/html", htmlBody)
	dialer := mail.NewDialer(s.config.SMTPHost, s.config.SMTPPort, s.config.SMTPUser, s.config.SMTPPassword)
	dialer.Timeout = 20 * time.Second
	if s.config.SMTPPort != 465 {
		dialer.StartTLSPolicy = mail.MandatoryStartTLS
	}
	return dialer.DialAndSend(message)
}

type emailRow struct{ Label, Value string }

type emailData struct {
	Name         string
	TicketNumber string
	Rows         []emailRow
}

var emailTmpl = template.Must(template.New("email").Parse(`<!DOCTYPE html>
<html lang="bn">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>টিকিট সম্পন্ন</title>
</head>
<body style="margin:0;padding:0;background:#eff6ff;font-family:'Hind Siliguri',Arial,sans-serif;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#eff6ff;padding:24px 12px;">
<tr><td align="center">
  <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:520px;background:#ffffff;border-radius:24px;overflow:hidden;box-shadow:0 10px 25px rgba(15,23,42,0.08);">
    <tr><td style="background-color:#10b981;background-image:linear-gradient(135deg,#10b981,#059669);padding:32px 24px;text-align:center;color:#ffffff;">
      <div style="font-size:44px;line-height:1;">✅</div>
      <h1 style="margin:14px 0 6px;font-size:24px;line-height:1.3;">আপনার টিকিট সম্পন্ন হয়েছে</h1>
      <p style="margin:0;font-size:15px;line-height:1.6;">প্রিয় {{.Name}}, আপনার অনুরোধটি সফলভাবে সম্পন্ন করা হয়েছে।</p>
    </td></tr>
    <tr><td style="padding:26px 24px 10px;text-align:center;">
      <div style="font-size:13px;color:#64748b;">টিকিট নম্বর</div>
      <div style="font-family:'Courier New',Courier,monospace;font-size:24px;font-weight:700;color:#2563eb;letter-spacing:1px;margin-top:6px;">{{.TicketNumber}}</div>
    </td></tr>
    <tr><td style="padding:8px 24px 0;"><div style="border-top:2px dashed #e2e8f0;height:0;line-height:0;">&nbsp;</div></td></tr>
    <tr><td style="padding:14px 24px 6px;">
      <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="font-size:15px;color:#334155;">
        {{range .Rows}}
        <tr>
          <td style="padding:9px 0;color:#64748b;white-space:nowrap;vertical-align:top;">{{.Label}}</td>
          <td style="padding:9px 0 9px 16px;text-align:right;font-weight:600;word-break:break-all;">{{.Value}}</td>
        </tr>
        {{end}}
        <tr>
          <td style="padding:9px 0;color:#64748b;">📊 স্ট্যাটাস</td>
          <td style="padding:9px 0;text-align:right;"><span style="display:inline-block;background:#d1fae5;color:#047857;padding:4px 14px;border-radius:999px;font-size:13px;font-weight:700;">✅ সম্পন্ন</span></td>
        </tr>
      </table>
    </td></tr>
    <tr><td style="padding:18px 24px 28px;text-align:center;font-size:12px;line-height:1.7;color:#94a3b8;">
      এটি একটি স্বয়ংক্রিয় ইমেইল — অনুগ্রহ করে রিপ্লাই করবেন না।<br>Auto Ticket System
    </td></tr>
  </table>
</td></tr>
</table>
</body>
</html>`))

func renderTicketEmail(t domain.Ticket, loc *time.Location) (htmlBody, textBody string, err error) {
	created := formatBanglaDateTime(t.CreatedAt.In(loc))
	all := []emailRow{
		{"👤 নাম", t.EmployeeName},
		{"🆔 আইডি", t.EmployeeID},
		{"🏢 বিভাগ", t.Department},
		{"সমস্যা", t.Problem},
		{"Assignee", t.AssignedTo},
		{"📧 ইমেইল", t.Email},
		{"📱 ফোন", t.Phone},
		{"🕐 তৈরি", created},
	}
	rows := make([]emailRow, 0, len(all))
	for _, r := range all {
		if strings.TrimSpace(r.Value) != "" {
			rows = append(rows, r)
		}
	}

	var buf bytes.Buffer
	if err = emailTmpl.Execute(&buf, emailData{Name: t.EmployeeName, TicketNumber: t.TicketNumber, Rows: rows}); err != nil {
		return "", "", err
	}

	var txt strings.Builder
	fmt.Fprintf(&txt, "প্রিয় %s,\n\nআপনার টিকিট সম্পন্ন হয়েছে।\n\nটিকিট নম্বর: %s\n", t.EmployeeName, t.TicketNumber)
	for _, r := range rows {
		fmt.Fprintf(&txt, "%s: %s\n", r.Label, r.Value)
	}
	txt.WriteString("স্ট্যাটাস: সম্পন্ন\n\n— Auto Ticket System")
	return buf.String(), txt.String(), nil
}

var bnDigits = strings.NewReplacer("0", "০", "1", "১", "2", "২", "3", "৩", "4", "৪", "5", "৫", "6", "৬", "7", "৭", "8", "৮", "9", "৯")

var bnMonths = [...]string{"জানুয়ারি", "ফেব্রুয়ারি", "মার্চ", "এপ্রিল", "মে", "জুন", "জুলাই", "আগস্ট", "সেপ্টেম্বর", "অক্টোবর", "নভেম্বর", "ডিসেম্বর"}

func formatBanglaDateTime(t time.Time) string {
	hour := t.Hour() % 12
	if hour == 0 {
		hour = 12
	}
	period := "AM"
	if t.Hour() >= 12 {
		period = "PM"
	}
	s := fmt.Sprintf("%d %s %d, %d:%02d %s", t.Day(), bnMonths[t.Month()-1], t.Year(), hour, t.Minute(), period)
	return bnDigits.Replace(s)
}
