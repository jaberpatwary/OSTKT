package groq

import (
	"strings"
	"testing"

	"ticket-system/internal/domain"
)

func TestTicketIntakeSystemPromptIncludesRosterAndLanguages(t *testing.T) {
	employees := []domain.Employee{
		{Name: "Rahim Uddin"},
		{Name: "Sumaiya Akter"},
	}
	assignees := []string{"Rupok Kumar Roy", "Towhedul Islam"}

	prompt := ticketIntakeSystemPrompt(employees, assignees)
	for _, expected := range []string{"Rahim Uddin", "Sumaiya Akter", "Rupok Kumar Roy", "Towhedul Islam", "Banglish", "বাংলা", "employee name → problem → assignee", "assign by"} {
		if !strings.Contains(prompt, expected) {
			t.Errorf("system prompt does not include %q", expected)
		}
	}
}
