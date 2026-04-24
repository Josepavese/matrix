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
	Documents      []string
	RequiredText   map[string][]string
	FileChecks     []fileCheck
	PatternBudgets []patternBudget
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
	PatternBudgets   int
	Failures         []string
}

type patternBudget struct {
	Name         string
	Roots        []string
	Patterns     []string
	AllowedFiles []string
	Max          int
	Reason       string
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
	budgets := make(map[string]*patternBudget)
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
			if strings.HasPrefix(section, "pattern_budget.") {
				name := strings.TrimPrefix(section, "pattern_budget.")
				budget := budgets[name]
				if budget == nil {
					budget = &patternBudget{Name: name}
					budgets[name] = budget
				}
				switch key {
				case "roots":
					budget.Roots, err = parseStringList(value)
				case "patterns":
					budget.Patterns, err = parseStringList(value)
				case "allowed_files":
					budget.AllowedFiles, err = parseStringList(value)
				case "max":
					budget.Max, err = parseInt(value)
				case "reason":
					budget.Reason, err = parseString(value)
				}
				if err != nil {
					return manifest{}, fmt.Errorf("%s:%d: %w", path, lineNo+1, err)
				}
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
	for _, budget := range budgets {
		m.PatternBudgets = append(m.PatternBudgets, *budget)
	}
	sort.Slice(m.PatternBudgets, func(i, j int) bool {
		return m.PatternBudgets[i].Name < m.PatternBudgets[j].Name
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

	for _, budget := range m.PatternBudgets {
		report.PatternBudgets++
		report.Failures = append(report.Failures, checkPatternBudget(root, budget)...)
	}

	return report
}

func checkPatternBudget(root string, budget patternBudget) []string {
	if len(budget.Roots) == 0 || len(budget.Patterns) == 0 {
		return []string{fmt.Sprintf("[pattern_budget.%s] roots and patterns are required", budget.Name)}
	}

	allowed := make(map[string]struct{}, len(budget.AllowedFiles))
	for _, path := range budget.AllowedFiles {
		allowed[filepath.ToSlash(filepath.Clean(path))] = struct{}{}
	}

	var matches []string
	count := 0
	for _, rootPath := range budget.Roots {
		fullRoot := filepath.Join(root, filepath.FromSlash(rootPath))
		if _, err := os.Stat(fullRoot); err != nil {
			return []string{fmt.Sprintf("[pattern_budget.%s] root missing: %s", budget.Name, rootPath)}
		}
		err := filepath.WalkDir(fullRoot, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				switch entry.Name() {
				case ".git", "dist", "vendor", "node_modules":
					return filepath.SkipDir
				default:
					return nil
				}
			}
			if !isTextGovernanceTarget(path) {
				return nil
			}

			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if _, ok := allowed[rel]; ok {
				return nil
			}

			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			content := string(data)
			for _, pattern := range budget.Patterns {
				occurrences := strings.Count(content, pattern)
				if occurrences == 0 {
					continue
				}
				count += occurrences
				matches = append(matches, fmt.Sprintf("%s contains %q %d time(s)", rel, pattern, occurrences))
			}
			return nil
		})
		if err != nil {
			return []string{fmt.Sprintf("[pattern_budget.%s] scan failed: %v", budget.Name, err)}
		}
	}

	if count <= budget.Max {
		return nil
	}
	failure := fmt.Sprintf("[pattern_budget.%s] count %d exceeds max %d", budget.Name, count, budget.Max)
	if budget.Reason != "" {
		failure += ": " + budget.Reason
	}
	for _, match := range matches {
		failure += "\n  - " + match
	}
	return []string{failure}
}

func printReport(report checkReport) {
	fmt.Println("Matrix Governance Check")
	fmt.Printf("documents: %d\n", report.DocumentsChecked)
	fmt.Printf("text contracts: %d\n", report.TextContracts)
	fmt.Printf("file gates: %d\n", report.FileGates)
	fmt.Printf("pattern budgets: %d\n", report.PatternBudgets)
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

func parseInt(value string) (int, error) {
	return strconv.Atoi(strings.TrimSpace(value))
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

func isTextGovernanceTarget(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go", ".sh", ".ps1", ".md", ".yml", ".yaml", ".toml", ".json", ".mod", ".sum":
		return true
	default:
		return false
	}
}
