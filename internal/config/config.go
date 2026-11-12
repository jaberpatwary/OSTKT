package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Port            string
	DatabaseURL     string
	SQLitePath      string
	ExcelPath       string
	ExcelSheet      string
	IncludeInactive bool
	Timezone        string
	GinMode         string
	AppUser         string
	AppPassword     string
	SMTPHost        string
	SMTPPort        int
	SMTPUser        string
	SMTPPassword    string
	SMTPFrom        string
	SMTPFromName    string
}

func Load() (Config, error) {
	_ = godotenv.Load()
	smtpPort, err := strconv.Atoi(value("SMTP_PORT", "587"))
	if err != nil {
		return Config{}, err
	}
	return Config{
		Port:            value("PORT", "8080"),
		DatabaseURL:     value("DATABASE_URL", ""),
		SQLitePath:      value("SQLITE_PATH", "tickets.db"),
		ExcelPath:       value("EXCEL_PATH", "employees.xlsx"),
		ExcelSheet:      value("EXCEL_SHEET", "Master File"),
		IncludeInactive: strings.EqualFold(value("INCLUDE_INACTIVE", "true"), "true"),
		Timezone:        value("APP_TIMEZONE", "Asia/Dhaka"),
		GinMode:         value("GIN_MODE", ""),
		AppUser:         value("APP_USER", ""),
		AppPassword:     value("APP_PASS", ""),
		SMTPHost:        value("SMTP_HOST", ""),
		SMTPPort:        smtpPort,
		SMTPUser:        value("SMTP_USER", ""),
		SMTPPassword:    value("SMTP_PASS", ""),
		SMTPFrom:        value("SMTP_FROM", value("SMTP_USER", "")),
		SMTPFromName:    value("SMTP_FROM_NAME", "Auto Ticket System"),
	}, nil
}

func value(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
