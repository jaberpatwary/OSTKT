package excel

import (
	"errors"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/xuri/excelize/v2"

	"ticket-system/internal/domain"
)

type Employee = domain.Employee
type EmployeeStore struct {
	mu              sync.RWMutex
	path            string
	sheet           string
	includeInactive bool
	modTime         time.Time
	list            []Employee
}

func NewEmployeeStore(path, sheet string, includeInactive bool) (*EmployeeStore, error) {
	s := &EmployeeStore{path: path, sheet: sheet, includeInactive: includeInactive}
	if err := s.reload(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *EmployeeStore) reload() error {
	info, err := os.Stat(s.path)
	if err != nil {
		return err
	}
	list, skipped, err := readEmployees(s.path, s.sheet, s.includeInactive)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.list = list
	s.modTime = info.ModTime()
	s.mu.Unlock()
	if skipped > 0 {
		log.Printf("📊 %d জন employee লোড হয়েছে (%s → '%s'), In-Active %d জন বাদ", len(list), s.path, s.sheet, skipped)
	} else {
		log.Printf("📊 %d জন employee লোড হয়েছে (%s → '%s')", len(list), s.path, s.sheet)
	}
	return nil
}

// snapshot — বর্তমান তালিকা দেয়; Excel পরিবর্তন হলে আগে রিলোড করে
func (s *EmployeeStore) snapshot() []Employee {
	if info, err := os.Stat(s.path); err == nil {
		s.mu.RLock()
		changed := !info.ModTime().Equal(s.modTime)
		s.mu.RUnlock()
		if changed {
			if err := s.reload(); err != nil {
				log.Printf("⚠️ Excel আবার পড়া যায়নি, আগের তালিকাই চলছে: %v", err)
			}
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.list // reload পুরো slice বদলায়, তাই পড়া নিরাপদ
}

// Search — নাম (বা Employee ID) দিয়ে partial + case-insensitive খোঁজ।
// যাদের নাম কোয়েরি দিয়ে শুরু, তারা আগে আসে।
func (s *EmployeeStore) Search(query string, limit int) []Employee {
	q := normalize(query)
	result := []Employee{}
	if q == "" {
		return result
	}
	phoneQuery := digitsOnly(query)
	var startsWith, contains, identifierMatch []Employee
	for _, e := range s.snapshot() {
		name := normalize(e.Name)
		switch {
		case strings.HasPrefix(name, q):
			startsWith = append(startsWith, e)
		case strings.Contains(name, q):
			contains = append(contains, e)
		case strings.Contains(strings.ToLower(e.EmployeeID), q), strings.Contains(strings.ToLower(e.Email), q):
			identifierMatch = append(identifierMatch, e)
		case phoneQuery != "" && strings.Contains(digitsOnly(e.Phone), phoneQuery):
			identifierMatch = append(identifierMatch, e)
		}
	}
	result = append(result, startsWith...)
	result = append(result, contains...)
	result = append(result, identifierMatch...)
	if len(result) > limit {
		result = result[:limit]
	}
	return result
}

func digitsOnly(value string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, value)
}

// Resolve — টিকিট বানানোর আগে নির্দিষ্ট employee খুঁজে বের করে।
// employeeID দিলে সেটা দিয়েই নিখুঁতভাবে মেলায় (একই নামে একাধিক জন থাকলে কাজে লাগে)।
// না দিলে: আগে হুবহু নাম, না পেলে partial match।
func (s *EmployeeStore) Resolve(name, employeeID string) []Employee {
	list := s.snapshot()

	if id := strings.TrimSpace(employeeID); id != "" {
		for _, e := range list {
			if strings.EqualFold(e.EmployeeID, id) {
				return []Employee{e}
			}
		}
		return nil
	}

	q := normalize(name)
	if q == "" {
		return nil
	}
	var exact, partial []Employee
	for _, e := range list {
		n := normalize(e.Name)
		if n == q {
			exact = append(exact, e)
		} else if strings.Contains(n, q) {
			partial = append(partial, e)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	return partial
}

// কলামের হেডার যেকোনো ক্রমে থাকতে পারে — নাম দেখে কলাম চেনা হয়
var headerAliases = map[string][]string{
	"name":   {"name", "employee name", "name (as per nid)"},
	"id":     {"hr id", "employeeid", "employee id", "emp id"},
	"dept":   {"department", "dept"},
	"email":  {"ds mail", "email", "office email", "official email"},
	"email2": {"personal email"},
	"phone":  {"contact no.", "contact no", "phone", "mobile"},
	"status": {"status", "active/inactive", "active/ inactive"},
}

func normalizeHeader(h string) string { return strings.ToLower(cleanSpace(h)) }

// findHeaderRow — প্রথম ১৫ সারির মধ্যে যে সারিতে Name আর HR ID দুটোই আছে, সেটাই header
// (Master File-এ উপরে "The Daily Star" / "Employee List" টাইটেল সারি থাকে)
func findHeaderRow(rows [][]string) (int, map[string]int, error) {
	for i := 0; i < len(rows) && i < 15; i++ {
		byHeader := map[string]int{}
		for c, h := range rows[i] {
			if key := normalizeHeader(h); key != "" {
				if _, dup := byHeader[key]; !dup {
					byHeader[key] = c
				}
			}
		}
		cols := map[string]int{}
		for field, aliases := range headerAliases {
			for _, a := range aliases {
				if c, ok := byHeader[a]; ok {
					cols[field] = c
					break
				}
			}
		}
		_, hasName := cols["name"]
		_, hasID := cols["id"]
		if hasName && hasID {
			return i, cols, nil
		}
	}
	return 0, nil, errors.New("header সারি পাওয়া যায়নি — 'Name' আর 'HR ID' কলাম লাগবে")
}

// findSheet — নাম মিলিয়ে শিট খোঁজে (বড়/ছোট হাতের অক্ষর ও আগে-পরের স্পেস উপেক্ষা করে)
func findSheet(f *excelize.File, want string) (string, error) {
	sheets := f.GetSheetList()
	for _, name := range sheets {
		if strings.EqualFold(strings.TrimSpace(name), strings.TrimSpace(want)) {
			return name, nil
		}
	}
	return "", fmt.Errorf("'%s' শিট পাওয়া যায়নি — ফাইলে আছে: %s", want, strings.Join(sheets, ", "))
}

// readEmployees — শুধু নির্দিষ্ট শিট (ডিফল্ট "Master File") পড়ে। বাকি শিট উপেক্ষা করা হয়।
// ফেরত দেয়: employee তালিকা, কতজন In-Active বাদ গেছে
func readEmployees(path, sheet string, includeInactive bool) ([]Employee, int, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, 0, fmt.Errorf("Excel খোলা যায়নি: %w", err)
	}
	defer f.Close()

	sheetName, err := findSheet(f, sheet)
	if err != nil {
		return nil, 0, err
	}
	rows, err := f.GetRows(sheetName)
	if err != nil {
		return nil, 0, fmt.Errorf("শিট পড়া যায়নি: %w", err)
	}
	headerIdx, cols, err := findHeaderRow(rows)
	if err != nil {
		return nil, 0, fmt.Errorf("'%s' শিট: %w", sheetName, err)
	}

	list := make([]Employee, 0, len(rows))
	skipped := 0
	for _, row := range rows[headerIdx+1:] {
		get := func(field string) string {
			c, ok := cols[field]
			if !ok || c >= len(row) {
				return ""
			}
			return row[c]
		}
		name := cleanSpace(get("name"))
		if name == "" {
			continue // খালি সারি বাদ
		}
		if !includeInactive && isInactive(get("status")) {
			skipped++
			continue
		}
		list = append(list, Employee{
			Name:       name,
			EmployeeID: cleanSpace(get("id")),
			Department: cleanSpace(get("dept")),
			Email:      firstEmail(get("email")), // শুধু অফিসিয়াল DS Mail; Personal Email কখনো নয়
			Phone:      normalizePhone(get("phone")),
		})
	}
	return list, skipped, nil
}

// "In-Active", "Inactive", "In-Active - Keeron" → true
func isInactive(status string) bool {
	s := strings.NewReplacer("-", "", " ", "", "_", "").Replace(strings.ToLower(status))
	return strings.HasPrefix(s, "inactive")
}

// Excel সেলে অদৃশ্য ক্যারেক্টার (LRM, zero-width space, BOM) থাকে — সেগুলো বাদ
var invisibleChars = strings.NewReplacer("\u200e", "", "\u200f", "", "\u200b", "", "\ufeff", "", "\u00a0", " ")

func cleanSpace(s string) string {
	return strings.Join(strings.Fields(invisibleChars.Replace(s)), " ")
}
func normalize(s string) string { return strings.ToLower(cleanSpace(s)) }

var emailRe = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)

// firstEmail — প্রথম বৈধ ইমেইল নেয়; "-", "0", শেষে ";" ইত্যাদি বাদ দেয়
func firstEmail(values ...string) string {
	for _, v := range values {
		if m := emailRe.FindString(v); m != "" {
			return strings.ToLower(m)
		}
	}
	return ""
}

var (
	phoneSeparators = regexp.MustCompile(`[\n,;/]+`)
	bdMobile        = regexp.MustCompile(`^01[3-9]\d{8}$`)
)

// normalizePhone — একাধিক নম্বর থাকলে প্রথম বৈধ মোবাইল নেয়, ড্যাশ/স্পেস বাদ দেয়,
// Excel সংখ্যা ধরে নেওয়ায় হারানো শুরুর 0 ফিরিয়ে আনে, +880 → 0
func normalizePhone(raw string) string {
	fallback := ""
	for _, part := range phoneSeparators.Split(raw, -1) {
		digits := strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, part)
		switch {
		case len(digits) == 13 && strings.HasPrefix(digits, "880"):
			digits = "0" + digits[3:]
		case len(digits) == 10 && digits[0] == '1':
			digits = "0" + digits
		}
		if bdMobile.MatchString(digits) {
			return digits
		}
		if fallback == "" && len(digits) >= 6 { // ল্যান্ডলাইন ইত্যাদি
			fallback = digits
		}
	}
	return fallback
}

// writeSampleExcel — স্যাম্পল ডেটাসহ employees.xlsx বানায় (go run . -sample-excel)
func writeSampleExcel(path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s আগে থেকেই আছে — মুছে তারপর আবার চালান", path)
	}
	f := excelize.NewFile()
	defer f.Close()

	// আসল ফাইলের মতোই: "Master File" শিট, একই কলামের নাম
	const sheet = "Master File"
	if err := f.SetSheetName("Sheet1", sheet); err != nil {
		return err
	}
	rows := [][]string{
		{"Status", "HR ID", "Name", "Designation", "Department", "Contact No.", "DS Mail", "Personal Email"},
		{"Active", "1001", "Rahim Uddin", "Executive", "IT", "01711111111", "rahim@company.com", ""},
		{"Active", "1002", "Karim Ahmed", "Senior Executive", "HR & Admin", "01822222222", "karim@company.com", ""},
		{"Active", "1003", "Fatema Begum", "Accountant", "Finance", "01544444444", "fatema@company.com", ""},
		{"Active", "1010", "Hasan Ali", "Officer", "Finance", "01366666666", "hasan.ali@company.com", ""},
		{"Active", "1011", "Hasan Ali", "Officer", "Sales", "01477777777", "-", "hasan.ali@gmail.com"},
		{"Active", "1020", "সাকিব হাসান", "Reporter", "Reporting", "01788888888", "sakib@company.com", ""},
		{"In-Active", "1030", "Rahim Mia", "Officer", "HR & Admin", "01933333333", "rahim.mia@company.com", ""},
	}

	textStyle, err := f.NewStyle(&excelize.Style{NumFmt: 49}) // 49 = "@" (Text), ফোনের শুরুর 0 থাকবে
	if err != nil {
		return err
	}
	headerStyle, err := f.NewStyle(&excelize.Style{
		Font:   &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill:   excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"2563EB"}},
		NumFmt: 49,
	})
	if err != nil {
		return err
	}
	if err := f.SetColStyle(sheet, "A:H", textStyle); err != nil {
		return err
	}
	if err := f.SetColWidth(sheet, "A", "H", 22); err != nil {
		return err
	}
	for r, row := range rows {
		for c, val := range row {
			cell, err := excelize.CoordinatesToCellName(c+1, r+1)
			if err != nil {
				return err
			}
			if err := f.SetCellStr(sheet, cell, val); err != nil {
				return err
			}
		}
	}
	if err := f.SetCellStyle(sheet, "A1", "H1", headerStyle); err != nil {
		return err
	}
	return f.SaveAs(path)
}

func WriteSampleWorkbook(path string) error {
	return writeSampleExcel(path)
}
