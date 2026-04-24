package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type manifest struct {
	Documents    []string
	RequiredText map[string][]string
	FileChecks   []fileCheck
}

type fileCheck struct {
	Section  string
	File     string
	Required []string
}

type checkReport struct {
	DocumentsChecked int
	TextContracts    int
	FileGates        int
	Failures         []string
}

func main() {
	manifestPath := flag.String("manifest", "governance/manifest.toml", "path to the governance manifest")
	root := flag.String("root", ".", "repository root")
	flag.Parse()

	m, err := loadManifest(*manifestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "governance manifest error: %v\n", err)
		os.Exit(1)
	}

	report := checkManifest(*root, m)
	printReport(report)
	if len(report.Failures) > 0 {
		os.Exit(1)
	}
}

func loadManifest(path string) (manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return manifest{}, err
	}

	m := manifest{
		RequiredText: make(map[string][]string),
	}
	checks := make(map[string]*fileCheck)
	section := ""

	lines := strings.Split(string(data), "\n")
	for lineNo := 0; lineNo < len(lines); lineNo++ {
		raw := lines[lineNo]
		line := strings.TrimSpace(stripComment(raw))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return manifest{}, fmt.Errorf("%s:%d: expected key = value", path, lineNo+1)
		}
		key := strings.TrimSpace(parts[0])
		key = strings.Trim(key, `"`)
		value := strings.TrimSpace(parts[1])
		if strings.HasPrefix(value, "[") && !strings.HasSuffix(value, "]") {
			for !strings.HasSuffix(strings.TrimSpace(value), "]") {
				lineNo++
				if lineNo >= len(lines) {
					return manifest{}, fmt.Errorf("%s:%d: unterminated string list", path, lineNo)
				}
				value += " " + strings.TrimSpace(stripComment(lines[lineNo]))
			}
		}

		switch section {
		case "documents":
			if key == "required" {
				m.Documents, err = parseStringList(value)
			}
		case "required_text":
			m.RequiredText[key], err = parseStringList(value)
		default:
			if section == "" || section == "metadata" {
				continue
			}
			check := checks[section]
			if check == nil {
				check = &fileCheck{Section: section}
				checks[section] = check
			}
			switch key {
			case "file":
				check.File, err = parseString(value)
			case "required":
				check.Required, err = parseStringList(value)
			}
		}
		if err != nil {
			return manifest{}, fmt.Errorf("%s:%d: %w", path, lineNo+1, err)
		}
	}

	for _, check := range checks {
		if check.File != "" || len(check.Required) > 0 {
			m.FileChecks = append(m.FileChecks, *check)
		}
	}
	sort.Slice(m.FileChecks, func(i, j int) bool {
		return m.FileChecks[i].Section < m.FileChecks[j].Section
	})

	return m, nil
}

func checkManifest(root string, m manifest) checkReport {
	report := checkReport{}

	for _, doc := range m.Documents {
		report.DocumentsChecked++
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(doc)))
		if err != nil {
			report.Failures = append(report.Failures, fmt.Sprintf("required document missing: %s", doc))
			continue
		}
		if strings.TrimSpace(string(data)) == "" {
			report.Failures = append(report.Failures, fmt.Sprintf("required document is empty: %s", doc))
		}
	}

	textFiles := sortedKeys(m.RequiredText)
	for _, path := range textFiles {
		report.TextContracts++
		content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			report.Failures = append(report.Failures, fmt.Sprintf("required text file missing: %s", path))
			continue
		}
		for _, token := range m.RequiredText[path] {
			if !strings.Contains(string(content), token) {
				report.Failures = append(report.Failures, fmt.Sprintf("%s must contain %q", path, token))
			}
		}
	}

	for _, check := range m.FileChecks {
		report.FileGates++
		content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(check.File)))
		if err != nil {
			report.Failures = append(report.Failures, fmt.Sprintf("[%s] file missing: %s", check.Section, check.File))
			continue
		}
		for _, token := range check.Required {
			if !strings.Contains(string(content), token) {
				report.Failures = append(report.Failures, fmt.Sprintf("[%s] %s must contain %q", check.Section, check.File, token))
			}
		}
	}

	return report
}

func printReport(report checkReport) {
	fmt.Println("Matrix Governance Check")
	fmt.Printf("documents: %d\n", report.DocumentsChecked)
	fmt.Printf("text contracts: %d\n", report.TextContracts)
	fmt.Printf("file gates: %d\n", report.FileGates)
	if len(report.Failures) == 0 {
		fmt.Println("failures: 0")
		fmt.Println("GOVERNANCE_CHECK_OK")
		return
	}

	fmt.Printf("failures: %d\n", len(report.Failures))
	for _, failure := range report.Failures {
		fmt.Printf("- %s\n", failure)
	}
}

func parseString(value string) (string, error) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, `"`) {
		return strconv.Unquote(value)
	}
	return value, nil
}

func parseStringList(value string) ([]string, error) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "[") || !strings.HasSuffix(value, "]") {
		return nil, fmt.Errorf("expected string list")
	}
	inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "["), "]"))
	if inner == "" {
		return nil, nil
	}

	parts := splitCommaSeparated(inner)
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		item, err := parseString(strings.TrimSpace(part))
		if err != nil {
			return nil, err
		}
		if item != "" {
			items = append(items, item)
		}
	}
	return items, nil
}

func splitCommaSeparated(value string) []string {
	var parts []string
	start := 0
	inString := false
	escaped := false
	for i, r := range value {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' && inString {
			escaped = true
			continue
		}
		if r == '"' {
			inString = !inString
			continue
		}
		if r == ',' && !inString {
			parts = append(parts, value[start:i])
			start = i + 1
		}
	}
	parts = append(parts, value[start:])
	return parts
}

func stripComment(line string) string {
	inString := false
	escaped := false
	for i, r := range line {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' && inString {
			escaped = true
			continue
		}
		if r == '"' {
			inString = !inString
			continue
		}
		if r == '#' && !inString {
			return line[:i]
		}
	}
	return line
}

func sortedKeys(values map[string][]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
