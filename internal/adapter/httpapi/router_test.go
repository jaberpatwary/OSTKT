package httpapi

import (
	"errors"
	"testing"

	"ticket-system/internal/adapter/groq"
	"ticket-system/internal/domain"
)

func TestContainsSpeechWords(t *testing.T) {
	tests := []struct {
		name string
		text string
		want bool
	}{
		{name: "empty", text: "", want: false},
		{name: "punctuation only", text: "...?!", want: false},
		{name: "english", text: "Hello", want: true},
		{name: "bangla", text: "হ্যালো", want: true},
		{name: "number", text: "123", want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := containsSpeechWords(test.text); got != test.want {
				t.Fatalf("containsSpeechWords(%q) = %v, want %v", test.text, got, test.want)
			}
		})
	}
}

func TestIsAssistantRateLimited(t *testing.T) {
	if !isAssistantRateLimited(errors.New("Groq API returned HTTP 429")) {
		t.Fatal("expected HTTP 429 to be classified as rate limited")
	}
	if isAssistantRateLimited(errors.New("Groq API returned HTTP 502")) {
		t.Fatal("HTTP 502 must not be classified as rate limited")
	}
}

func TestRelevantAssistantEmployeesMatchesNameAndBanglishTypos(t *testing.T) {
	employees := []domain.Employee{
		{Name: "Sumaiya Akter", EmployeeID: "101"},
		{Name: "Jaber Uddin", EmployeeID: "102"},
		{Name: "Towhedul Islam", EmployeeID: "103"},
	}
	messages := []groq.Message{{Role: "user", Content: "Sumaiyar laptop e network kaj korche na, assign Rupok ke"}}

	got := relevantAssistantEmployees(messages, employees, 8)
	if len(got) == 0 || got[0].Name != "Sumaiya Akter" {
		t.Fatalf("relevantAssistantEmployees() = %#v, want Sumaiya Akter among top candidates", got)
	}
	for _, employee := range got {
		if employee.Name == "Jaber Uddin" {
			t.Fatalf("unrelated employee was included: %#v", got)
		}
	}
}

func TestRelevantAssistantEmployeesIgnoresAssistantText(t *testing.T) {
	employees := []domain.Employee{{Name: "Rahim Uddin"}}
	messages := []groq.Message{
		{Role: "assistant", Content: "Rahim Uddin has a laptop issue"},
		{Role: "user", Content: "Hello, how are you?"},
	}
	if got := relevantAssistantEmployees(messages, employees, 8); len(got) != 0 {
		t.Fatalf("relevantAssistantEmployees() = %#v, want no candidates", got)
	}
}
