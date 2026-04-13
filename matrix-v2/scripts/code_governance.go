package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

type config struct {
	Budgets struct {
		PackageLOC     int
		FileLOC        int
		FunctionLOC    int
		FunctionParams int
	}
	Quality struct {
		AvgFunctionLOCWarn float64
		P95FunctionLOCWarn float64
		BranchPointsWarn   int
		LongFunctionRatio  float64
	}
	Scope struct {
		Roots       []string
		ExcludeDirs []string
	}
	PackageOverrides       map[string]int
	FileOverrides          map[string]int
	FunctionOverrides      map[string]int
	FunctionParamOverrides map[string]int
}

type fileReport struct {
	Path    string
	Package string
	LOC     int
	Test    bool
}

type funcReport struct {
	Name         string
	Qualified    string
	Path         string
	Package      string
	LOC          int
	Params       int
	BranchPoints int
	Test         bool
}

type packageReport struct {
	Name    string
	LOC     int
	TestLOC int
	Files   int
}

func main() {
	configPath := flag.String("config", "code-governance.toml", "path to governance config")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	files, funcs, pkgs, err := analyze(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "analysis error: %v\n", err)
		os.Exit(1)
	}

	failures, warnings := evaluate(cfg, files, funcs, pkgs)
	printReport(cfg, files, funcs, pkgs, failures, warnings)

	if len(failures) > 0 {
		os.Exit(1)
	}
}

func loadConfig(path string) (config, error) {
	cfg := config{}
	file, err := os.Open(path)
	if err != nil {
		return cfg, err
	}
	defer file.Close()

	section := ""
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.Trim(line, "[]")
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return cfg, fmt.Errorf("invalid config line: %s", line)
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		switch section {
		case "budgets":
			switch key {
			case "package_loc":
				cfg.Budgets.PackageLOC, err = atoi(val)
			case "file_loc":
				cfg.Budgets.FileLOC, err = atoi(val)
			case "function_loc":
				cfg.Budgets.FunctionLOC, err = atoi(val)
			case "function_params":
				cfg.Budgets.FunctionParams, err = atoi(val)
			}
		case "quality":
			switch key {
			case "avg_function_loc_warn":
				cfg.Quality.AvgFunctionLOCWarn, err = atof(val)
			case "p95_function_loc_warn":
				cfg.Quality.P95FunctionLOCWarn, err = atof(val)
			case "branch_points_warn":
				cfg.Quality.BranchPointsWarn, err = atoi(val)
			case "long_function_ratio_warn":
				cfg.Quality.LongFunctionRatio, err = atof(val)
			}
		case "scope":
			switch key {
			case "roots":
				cfg.Scope.Roots, err = parseStringList(val)
			case "exclude_dirs":
				cfg.Scope.ExcludeDirs, err = parseStringList(val)
			}
		case "package_overrides":
			err = ensureMap(&cfg.PackageOverrides, key, val)
		case "file_overrides":
			err = ensureMap(&cfg.FileOverrides, key, val)
		case "function_overrides":
			err = ensureMap(&cfg.FunctionOverrides, key, val)
		case "function_param_overrides":
			err = ensureMap(&cfg.FunctionParamOverrides, key, val)
		}
		if err != nil {
			return cfg, fmt.Errorf("invalid value for %s.%s: %w", section, key, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return cfg, err
	}
	if len(cfg.Scope.Roots) == 0 {
		return cfg, errors.New("scope.roots must not be empty")
	}
	return cfg, nil
}

func analyze(cfg config) ([]fileReport, []funcReport, map[string]*packageReport, error) {
	var files []fileReport
	var funcs []funcReport
	pkgs := map[string]*packageReport{}
	fset := token.NewFileSet()

	for _, root := range cfg.Scope.Roots {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				for _, ex := range cfg.Scope.ExcludeDirs {
					if path == ex || strings.HasPrefix(path, ex+string(os.PathSeparator)) {
						return filepath.SkipDir
					}
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			report, fnReports, err := analyzeFile(fset, path)
			if err != nil {
				return err
			}
			files = append(files, report)
			funcs = append(funcs, fnReports...)

			pkg := pkgs[report.Package]
			if pkg == nil {
				pkg = &packageReport{Name: report.Package}
				pkgs[report.Package] = pkg
			}
			pkg.Files++
			if report.Test {
				pkg.TestLOC += report.LOC
			} else {
				pkg.LOC += report.LOC
			}
			return nil
		})
		if err != nil {
			return nil, nil, nil, err
		}
	}

	sort.Slice(files, func(i, j int) bool { return files[i].LOC > files[j].LOC })
	sort.Slice(funcs, func(i, j int) bool { return funcs[i].LOC > funcs[j].LOC })
	return files, funcs, pkgs, nil
}

func analyzeFile(fset *token.FileSet, path string) (fileReport, []funcReport, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return fileReport{}, nil, err
	}
	node, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return fileReport{}, nil, err
	}

	lines := strings.Split(string(src), "\n")
	report := fileReport{
		Path:    path,
		Package: filepath.Dir(path),
		LOC:     countMeaningfulLines(lines, 1, len(lines)),
		Test:    strings.HasSuffix(path, "_test.go"),
	}

	var funcs []funcReport
	for _, decl := range node.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		start := fset.Position(fn.Pos()).Line
		end := fset.Position(fn.End()).Line
		name := fn.Name.Name
		if fn.Recv != nil && len(fn.Recv.List) > 0 {
			name = receiverName(fn.Recv.List[0].Type) + "." + name
		}
		funcs = append(funcs, funcReport{
			Name:         fn.Name.Name,
			Qualified:    name,
			Path:         path,
			Package:      report.Package,
			LOC:          countMeaningfulLines(lines, start, end),
			Params:       countParams(fn.Type),
			BranchPoints: countBranchPoints(fn.Body),
			Test:         report.Test,
		})
	}

	return report, funcs, nil
}

func evaluate(cfg config, files []fileReport, funcs []funcReport, pkgs map[string]*packageReport) ([]string, []string) {
	var failures []string
	var warnings []string

	for _, pkg := range pkgs {
		budget := overrideOrDefault(cfg.PackageOverrides, pkg.Name, cfg.Budgets.PackageLOC)
		if pkg.LOC > budget {
			failures = append(failures, fmt.Sprintf("package budget exceeded: %s has %d LOC (budget %d)", pkg.Name, pkg.LOC, budget))
		}
	}
	for _, file := range files {
		if file.Test {
			continue
		}
		budget := overrideOrDefault(cfg.FileOverrides, file.Path, cfg.Budgets.FileLOC)
		if file.LOC > budget {
			failures = append(failures, fmt.Sprintf("file budget exceeded: %s has %d LOC (budget %d)", file.Path, file.LOC, budget))
		}
	}

	var prodFuncs []funcReport
	longFuncs := 0
	for _, fn := range funcs {
		if fn.Test {
			continue
		}
		prodFuncs = append(prodFuncs, fn)
		locBudget := overrideOrDefault(cfg.FunctionOverrides, fn.Path+"::"+fn.Qualified, cfg.Budgets.FunctionLOC)
		if fn.LOC > locBudget {
			failures = append(failures, fmt.Sprintf("function budget exceeded: %s in %s has %d LOC (budget %d)", fn.Qualified, fn.Path, fn.LOC, locBudget))
		}
		paramBudget := overrideOrDefault(cfg.FunctionParamOverrides, fn.Path+"::"+fn.Qualified, cfg.Budgets.FunctionParams)
		if fn.Params > paramBudget {
			failures = append(failures, fmt.Sprintf("function params exceeded: %s in %s has %d params (budget %d)", fn.Qualified, fn.Path, fn.Params, paramBudget))
		}
		if float64(fn.LOC) > cfg.Quality.P95FunctionLOCWarn {
			longFuncs++
		}
		if fn.BranchPoints > cfg.Quality.BranchPointsWarn {
			warnings = append(warnings, fmt.Sprintf("high branch complexity: %s in %s has %d branch points (warn %d)", fn.Qualified, fn.Path, fn.BranchPoints, cfg.Quality.BranchPointsWarn))
		}
	}

	if len(prodFuncs) > 0 {
		avg := averageLOC(prodFuncs)
		p95 := percentileLOC(prodFuncs, 0.95)
		ratio := float64(longFuncs) / float64(len(prodFuncs))
		if avg > cfg.Quality.AvgFunctionLOCWarn {
			warnings = append(warnings, fmt.Sprintf("average function LOC is %.2f (warn %.2f)", avg, cfg.Quality.AvgFunctionLOCWarn))
		}
		if p95 > cfg.Quality.P95FunctionLOCWarn {
			warnings = append(warnings, fmt.Sprintf("p95 function LOC is %.2f (warn %.2f)", p95, cfg.Quality.P95FunctionLOCWarn))
		}
		if ratio > cfg.Quality.LongFunctionRatio {
			warnings = append(warnings, fmt.Sprintf("long function ratio is %.2f (warn %.2f)", ratio, cfg.Quality.LongFunctionRatio))
		}
	}

	sort.Strings(failures)
	sort.Strings(warnings)
	return failures, warnings
}

func printReport(cfg config, files []fileReport, funcs []funcReport, pkgs map[string]*packageReport, failures, warnings []string) {
	fmt.Println("Matrix Code Governance Report")
	fmt.Println()
	fmt.Printf("Budgets: package=%d file=%d function=%d params=%d\n", cfg.Budgets.PackageLOC, cfg.Budgets.FileLOC, cfg.Budgets.FunctionLOC, cfg.Budgets.FunctionParams)
	fmt.Printf("Quality warnings: avg_func_loc<=%.2f p95_func_loc<=%.2f branch_points<=%d long_function_ratio<=%.2f\n", cfg.Quality.AvgFunctionLOCWarn, cfg.Quality.P95FunctionLOCWarn, cfg.Quality.BranchPointsWarn, cfg.Quality.LongFunctionRatio)
	fmt.Println()

	fmt.Println("Top Packages:")
	pkgList := make([]packageReport, 0, len(pkgs))
	for _, pkg := range pkgs {
		pkgList = append(pkgList, *pkg)
	}
	sort.Slice(pkgList, func(i, j int) bool { return pkgList[i].LOC > pkgList[j].LOC })
	for i, pkg := range pkgList {
		if i == 5 {
			break
		}
		fmt.Printf("- %s: prod_loc=%d test_loc=%d files=%d\n", pkg.Name, pkg.LOC, pkg.TestLOC, pkg.Files)
	}
	fmt.Println()

	fmt.Println("Top Files:")
	fileCount := 0
	for _, file := range files {
		if file.Test {
			continue
		}
		fmt.Printf("- %s: loc=%d\n", file.Path, file.LOC)
		fileCount++
		if fileCount == 5 {
			break
		}
	}
	fmt.Println()

	fmt.Println("Top Functions:")
	funcCount := 0
	for _, fn := range funcs {
		if fn.Test {
			continue
		}
		fmt.Printf("- %s (%s): loc=%d params=%d branch_points=%d\n", fn.Qualified, fn.Path, fn.LOC, fn.Params, fn.BranchPoints)
		funcCount++
		if funcCount == 8 {
			break
		}
	}
	fmt.Println()

	if len(failures) == 0 {
		fmt.Println("Hard Budget Failures: none")
	} else {
		fmt.Println("Hard Budget Failures:")
		for _, failure := range failures {
			fmt.Printf("- %s\n", failure)
		}
	}
	fmt.Println()

	if len(warnings) == 0 {
		fmt.Println("Quality Warnings: none")
	} else {
		fmt.Println("Quality Warnings:")
		for _, warning := range warnings {
			fmt.Printf("- %s\n", warning)
		}
	}
}

func countMeaningfulLines(lines []string, start, end int) int {
	if start < 1 {
		start = 1
	}
	if end > len(lines) {
		end = len(lines)
	}
	count := 0
	inBlockComment := false
	for i := start - 1; i < end; i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if inBlockComment {
			if strings.Contains(line, "*/") {
				inBlockComment = false
			}
			continue
		}
		if strings.HasPrefix(line, "//") {
			continue
		}
		if strings.HasPrefix(line, "/*") {
			if !strings.Contains(line, "*/") {
				inBlockComment = true
			}
			continue
		}
		count++
	}
	return count
}

func countParams(fn *ast.FuncType) int {
	if fn.Params == nil {
		return 0
	}
	total := 0
	for _, field := range fn.Params.List {
		if len(field.Names) == 0 {
			total++
			continue
		}
		total += len(field.Names)
	}
	return total
}

func countBranchPoints(body *ast.BlockStmt) int {
	points := 0
	ast.Inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
			points++
		case *ast.CaseClause:
			points += len(x.List)
		case *ast.BinaryExpr:
			if x.Op.String() == "&&" || x.Op.String() == "||" {
				points++
			}
		}
		return true
	})
	return points
}

func receiverName(expr ast.Expr) string {
	switch v := expr.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.StarExpr:
		return receiverName(v.X)
	default:
		return "recv"
	}
}

func averageLOC(funcs []funcReport) float64 {
	total := 0
	for _, fn := range funcs {
		total += fn.LOC
	}
	return float64(total) / float64(len(funcs))
}

func percentileLOC(funcs []funcReport, p float64) float64 {
	if len(funcs) == 0 {
		return 0
	}
	values := make([]int, 0, len(funcs))
	for _, fn := range funcs {
		values = append(values, fn.LOC)
	}
	sort.Ints(values)
	index := int(float64(len(values)-1) * p)
	return float64(values[index])
}

func atoi(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	n := 0
	for _, r := range raw {
		if !unicode.IsDigit(r) {
			return 0, fmt.Errorf("not an integer: %s", raw)
		}
		n = n*10 + int(r-'0')
	}
	return n, nil
}

func atof(raw string) (float64, error) {
	raw = strings.TrimSpace(raw)
	var intPart, fracPart float64
	var fracDiv float64 = 1
	seenDot := false
	for _, r := range raw {
		if r == '.' {
			if seenDot {
				return 0, fmt.Errorf("not a float: %s", raw)
			}
			seenDot = true
			continue
		}
		if !unicode.IsDigit(r) {
			return 0, fmt.Errorf("not a float: %s", raw)
		}
		digit := float64(r - '0')
		if !seenDot {
			intPart = intPart*10 + digit
		} else {
			fracDiv *= 10
			fracPart += digit / fracDiv
		}
	}
	return intPart + fracPart, nil
}

func parseStringList(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "[") || !strings.HasSuffix(raw, "]") {
		return nil, fmt.Errorf("not a string list: %s", raw)
	}
	raw = strings.TrimPrefix(strings.TrimSuffix(raw, "]"), "[")
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}, nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		part = strings.Trim(part, `"`)
		if part != "" {
			out = append(out, part)
		}
	}
	return out, nil
}

func ensureMap(target *map[string]int, key, val string) error {
	if *target == nil {
		*target = map[string]int{}
	}
	n, err := atoi(val)
	if err != nil {
		return err
	}
	(*target)[strings.Trim(key, `"`)] = n
	return nil
}

func overrideOrDefault(overrides map[string]int, key string, fallback int) int {
	if overrides == nil {
		return fallback
	}
	if n, ok := overrides[key]; ok {
		return n
	}
	return fallback
}
