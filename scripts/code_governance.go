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
	"regexp"
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
	WarningBudget struct {
		MaxTotalWarnings int
		MaxBranchPoints  int
	}
	Scope struct {
		Roots       []string
		ExcludeDirs []string
	}
	TestPolicy struct {
		// MinTestFunctions is the floor every package with production code must
		// reach. Test* and Fuzz* count; benchmarks and examples do not, because
		// they verify nothing.
		MinTestFunctions int
		// MinBehaviorTests requires at least this many tests that assert a
		// rejection, failure, or degraded path. Happy-path-only packages are the
		// ones that break in production.
		MinBehaviorTests int
		// BehaviorMinProdLOC gates the behaviour minimum to packages that carry
		// real logic. A package that only declares data has no rejection path to
		// assert, and demanding one would be theatre.
		BehaviorMinProdLOC int
		// BehaviorNamePattern is the SSOT list of name fragments that mark a
		// behaviour test, so the rule is tunable without touching the checker.
		BehaviorNamePattern string
		// MinTestLOCRatio and RatioMinProdLOC apply a per-package test/production
		// line ratio, but only to packages big enough for the ratio to mean
		// something.
		MinTestLOCRatio float64
		RatioMinProdLOC int
		// FuzzRequiredPackages must each carry at least one fuzz target: they
		// decode bytes that arrive from outside the process.
		FuzzRequiredPackages []string
		// InteropRequiredPackages must each carry at least one end-to-end test
		// against a peer, because their contract is a wire protocol.
		InteropRequiredPackages []string
		// Exemptions records packages where a minimum cannot be met yet. The
		// value must carry a date and a reason; the exemption is printed, and a
		// package that no longer needs one is reported so the list can shrink.
		Exemptions map[string]string
		// Baselines record, per package, the value reached today for a metric
		// that is still below the policy minimum. A baseline is a floor that may
		// only rise: it keeps legacy debt visible and prevents regression while
		// the absolute minimum applies to every package that is not listed.
		BaselineTestFunctions map[string]int
		BaselineBehaviorTests map[string]int
		BaselineTestLOCRatio  map[string]float64
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
	Name          string
	LOC           int
	TestLOC       int
	Files         int
	TestFiles     int
	TestFunctions int
	BehaviorTests int
	FuzzTargets   int
	InteropTests  int
}

func main() {
	configPath := flag.String("config", "code-governance.toml", "path to governance config")
	printBaseline := flag.Bool("print-test-baseline", false, "print the test baseline block for the current tree and exit")
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

	if *printBaseline {
		printTestBaseline(cfg, pkgs)
		return
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
	defer func() { _ = file.Close() }()

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
		case "warning_budget":
			switch key {
			case "max_total_warnings":
				cfg.WarningBudget.MaxTotalWarnings, err = atoi(val)
			case "max_branch_points":
				cfg.WarningBudget.MaxBranchPoints, err = atoi(val)
			}
		case "scope":
			switch key {
			case "roots":
				cfg.Scope.Roots, err = parseStringList(val)
			case "exclude_dirs":
				cfg.Scope.ExcludeDirs, err = parseStringList(val)
			}
		case "test_policy":
			switch key {
			case "min_test_functions_per_package":
				cfg.TestPolicy.MinTestFunctions, err = atoi(val)
			case "min_behavior_tests_per_package":
				cfg.TestPolicy.MinBehaviorTests, err = atoi(val)
			case "behavior_min_prod_loc":
				cfg.TestPolicy.BehaviorMinProdLOC, err = atoi(val)
			case "behavior_name_pattern":
				cfg.TestPolicy.BehaviorNamePattern, err = parseString(val)
			case "min_test_loc_ratio":
				cfg.TestPolicy.MinTestLOCRatio, err = atof(val)
			case "ratio_min_prod_loc":
				cfg.TestPolicy.RatioMinProdLOC, err = atoi(val)
			case "fuzz_required_packages":
				cfg.TestPolicy.FuzzRequiredPackages, err = parseStringList(val)
			case "interop_required_packages":
				cfg.TestPolicy.InteropRequiredPackages, err = parseStringList(val)
			}
		case "test_policy.exemptions":
			err = ensureTestExemption(&cfg.TestPolicy.Exemptions, key, val)
		case "test_policy.baseline_test_functions":
			err = ensureTestBaselineInt(&cfg.TestPolicy.BaselineTestFunctions, key, val)
		case "test_policy.baseline_behavior_tests":
			err = ensureTestBaselineInt(&cfg.TestPolicy.BaselineBehaviorTests, key, val)
		case "test_policy.baseline_ratio":
			err = ensureTestBaselineFloat(&cfg.TestPolicy.BaselineTestLOCRatio, key, val)
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
				pkg.TestFiles++
			} else {
				pkg.LOC += report.LOC
			}
			return nil
		})
		if err != nil {
			return nil, nil, nil, err
		}
	}

	classifyTests(pkgs, funcs, behaviorPattern(cfg))
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

// behaviorPattern compiles the configured behaviour-test name fragments. An
// empty pattern disables the behaviour minimum rather than matching everything.
func behaviorPattern(cfg config) *regexp.Regexp {
	pattern := strings.TrimSpace(cfg.TestPolicy.BehaviorNamePattern)
	if pattern == "" {
		return nil
	}
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid test_policy.behavior_name_pattern %q: %v\n", pattern, err)
		os.Exit(1)
	}
	return compiled
}

// classifyTests fills the per-package test inventory. A test is classified as
// interop when its file is an interop/integration file, and as a behaviour test
// when either the file is adversarial or its name names a rejection or failure
// path. A behaviour test also counts as a regular test function.
func classifyTests(pkgs map[string]*packageReport, funcs []funcReport, behavior *regexp.Regexp) {
	for _, fn := range funcs {
		if !fn.Test {
			continue
		}
		pkg := pkgs[fn.Package]
		if pkg == nil {
			continue
		}
		switch {
		case strings.HasPrefix(fn.Name, "Fuzz"):
			pkg.FuzzTargets++
			pkg.TestFunctions++
		case strings.HasPrefix(fn.Name, "Test"):
			pkg.TestFunctions++
			if strings.Contains(fn.Path, "interop") || strings.Contains(fn.Path, "integration") {
				pkg.InteropTests++
			}
			if behavior != nil && (strings.Contains(fn.Path, "adversarial") || behavior.MatchString(fn.Name)) {
				pkg.BehaviorTests++
			}
		}
	}
}

// evaluateTestPolicy enforces the per-package test minima. A package may sit on
// a recorded baseline instead of the absolute minimum, but the baseline is a
// floor: dropping below it fails, rising above it must be recorded, and a
// baseline that is no longer needed is reported so the debt list shrinks.
func evaluateTestPolicy(cfg config, pkgs map[string]*packageReport) ([]string, []string) {
	var failures []string
	var warnings []string
	policy := cfg.TestPolicy
	if policy.MinTestFunctions == 0 && policy.MinBehaviorTests == 0 && policy.MinTestLOCRatio == 0 &&
		len(policy.FuzzRequiredPackages) == 0 && len(policy.InteropRequiredPackages) == 0 {
		return nil, nil
	}
	names := make([]string, 0, len(pkgs))
	for name := range pkgs {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		pkg := pkgs[name]
		if pkg.LOC == 0 {
			continue
		}
		short := trimPackageName(name)
		if reason, ok := policy.Exemptions[name]; ok {
			if !regexp.MustCompile(`20[0-9]{2}-[0-9]{2}-[0-9]{2}`).MatchString(reason) {
				failures = append(failures, fmt.Sprintf("test exemption for %s must carry an ISO date and a reason", short))
			}
			continue
		}
		// Test function floor.
		if _, recorded := policy.BaselineTestFunctions[name]; recorded {
			baseline := policy.BaselineTestFunctions[name]
			if pkg.TestFunctions < baseline {
				failures = append(failures, fmt.Sprintf("test baseline regressed: %s has %d test functions (baseline %d)", short, pkg.TestFunctions, baseline))
			}
			if pkg.TestFunctions > baseline {
				warnings = append(warnings, fmt.Sprintf("test baseline can be raised for %s: %d to %d", short, baseline, pkg.TestFunctions))
			}
			if baseline >= policy.MinTestFunctions && policy.MinTestFunctions > 0 {
				warnings = append(warnings, fmt.Sprintf("test baseline for %s is at or above the policy minimum and can be deleted", short))
			}
		} else if policy.MinTestFunctions > 0 && pkg.TestFunctions < policy.MinTestFunctions {
			failures = append(failures, fmt.Sprintf("test minimum not met: %s has %d test functions (minimum %d)", short, pkg.TestFunctions, policy.MinTestFunctions))
		}

		// Behaviour floor, gated to packages that carry real logic.
		if policy.MinBehaviorTests > 0 && pkg.LOC >= policy.BehaviorMinProdLOC {
			if _, recorded := policy.BaselineBehaviorTests[name]; recorded {
				baseline := policy.BaselineBehaviorTests[name]
				if pkg.BehaviorTests < baseline {
					failures = append(failures, fmt.Sprintf("behaviour baseline regressed: %s has %d rejection/failure tests (baseline %d)", short, pkg.BehaviorTests, baseline))
				}
				if pkg.BehaviorTests > baseline {
					warnings = append(warnings, fmt.Sprintf("behaviour baseline can be raised for %s: %d to %d", short, baseline, pkg.BehaviorTests))
				}
			} else if pkg.BehaviorTests < policy.MinBehaviorTests {
				failures = append(failures, fmt.Sprintf("behaviour test minimum not met: %s has %d rejection/failure tests (minimum %d)", short, pkg.BehaviorTests, policy.MinBehaviorTests))
			}
		}

		// Test/production ratio floor.
		if policy.MinTestLOCRatio > 0 && pkg.LOC >= policy.RatioMinProdLOC {
			ratio := float64(pkg.TestLOC) / float64(pkg.LOC)
			if _, recorded := policy.BaselineTestLOCRatio[name]; recorded {
				baseline := policy.BaselineTestLOCRatio[name]
				if ratio+0.005 < baseline {
					failures = append(failures, fmt.Sprintf("test ratio baseline regressed: %s is at %.2f (baseline %.2f)", short, ratio, baseline))
				}
				if baseline >= policy.MinTestLOCRatio {
					warnings = append(warnings, fmt.Sprintf("test ratio baseline for %s is at or above the policy minimum and can be deleted", short))
				}
			} else if ratio < policy.MinTestLOCRatio {
				failures = append(failures, fmt.Sprintf("test ratio below minimum: %s has %.2f test/production lines (minimum %.2f)", short, ratio, policy.MinTestLOCRatio))
			}
		}
	}

	for _, name := range policy.FuzzRequiredPackages {
		if pkg := pkgs[name]; pkg == nil || pkg.FuzzTargets == 0 {
			failures = append(failures, fmt.Sprintf("fuzz target missing: %s decodes external input and must carry one", trimPackageName(name)))
		}
	}
	for _, name := range policy.InteropRequiredPackages {
		if pkg := pkgs[name]; pkg == nil || pkg.InteropTests == 0 {
			failures = append(failures, fmt.Sprintf("interop test missing: %s defines a wire contract and must carry an end-to-end test", trimPackageName(name)))
		}
	}

	for name, reason := range policy.Exemptions {
		pkg := pkgs[name]
		if pkg == nil {
			warnings = append(warnings, fmt.Sprintf("test exemption for %s matches no package (stale entry: %s)", trimPackageName(name), reason))
		}
	}
	return failures, warnings
}

// trimPackageName shortens a relative package path for reports.
func trimPackageName(name string) string {
	return strings.TrimPrefix(strings.TrimPrefix(name, "./"), "./")
}

func ensureTestExemption(target *map[string]string, key, val string) error {
	if *target == nil {
		*target = map[string]string{}
	}
	reason, err := parseString(val)
	if err != nil {
		return err
	}
	(*target)[strings.Trim(key, `"`)] = reason
	return nil
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

	failures = append(failures, evaluateWarningBudget(cfg, warnings, prodFuncs)...)

	testFailures, testWarnings := evaluateTestPolicy(cfg, pkgs)
	failures = append(failures, testFailures...)
	warnings = append(warnings, testWarnings...)

	sort.Strings(failures)
	sort.Strings(warnings)
	return failures, warnings
}

func evaluateWarningBudget(cfg config, warnings []string, prodFuncs []funcReport) []string {
	var failures []string
	if cfg.WarningBudget.MaxTotalWarnings > 0 && len(warnings) > cfg.WarningBudget.MaxTotalWarnings {
		failures = append(failures, fmt.Sprintf("warning budget exceeded: %d quality warnings (budget %d)", len(warnings), cfg.WarningBudget.MaxTotalWarnings))
	}
	if cfg.WarningBudget.MaxBranchPoints > 0 {
		fn, maxBranchPoints := maxBranchPointFunction(prodFuncs)
		if maxBranchPoints > cfg.WarningBudget.MaxBranchPoints {
			failures = append(failures, fmt.Sprintf("branch severity budget exceeded: %s in %s has %d branch points (budget %d)", fn.Qualified, fn.Path, maxBranchPoints, cfg.WarningBudget.MaxBranchPoints))
		}
	}
	return failures
}

func maxBranchPointFunction(funcs []funcReport) (funcReport, int) {
	var maxFn funcReport
	maxBranchPoints := 0
	for _, fn := range funcs {
		if fn.BranchPoints <= maxBranchPoints {
			continue
		}
		maxFn = fn
		maxBranchPoints = fn.BranchPoints
	}
	return maxFn, maxBranchPoints
}

// printTestBaseline emits the baseline block for every package that is still
// below a policy minimum. Raising a baseline after real improvement is a
// copy-paste of this output; lowering one is a deliberate, reviewed act.
func printTestBaseline(cfg config, pkgs map[string]*packageReport) {
	policy := cfg.TestPolicy
	names := make([]string, 0, len(pkgs))
	for name := range pkgs {
		names = append(names, name)
	}
	sort.Strings(names)

	var functions, behaviour, ratios []string
	for _, name := range names {
		pkg := pkgs[name]
		if pkg.LOC == 0 {
			continue
		}
		if _, exempt := policy.Exemptions[name]; exempt {
			continue
		}
		quoted := `"` + name + `"`
		if policy.MinTestFunctions > 0 && pkg.TestFunctions < policy.MinTestFunctions {
			functions = append(functions, fmt.Sprintf("%s = %d", quoted, pkg.TestFunctions))
		}
		if policy.MinBehaviorTests > 0 && pkg.LOC >= policy.BehaviorMinProdLOC && pkg.BehaviorTests < policy.MinBehaviorTests {
			behaviour = append(behaviour, fmt.Sprintf("%s = %d", quoted, pkg.BehaviorTests))
		}
		if policy.MinTestLOCRatio > 0 && pkg.LOC >= policy.RatioMinProdLOC {
			ratio := float64(pkg.TestLOC) / float64(pkg.LOC)
			if ratio < policy.MinTestLOCRatio {
				ratios = append(ratios, fmt.Sprintf("%s = %.2f", quoted, ratio))
			}
		}
	}
	printSection := func(header string, lines []string) {
		fmt.Println(header)
		if len(lines) == 0 {
			fmt.Println("# (none)")
		}
		for _, line := range lines {
			fmt.Println(line)
		}
		fmt.Println()
	}
	printSection("[test_policy.baseline_test_functions]", functions)
	printSection("[test_policy.baseline_behavior_tests]", behaviour)
	printSection("[test_policy.baseline_ratio]", ratios)
}

// printTestInventory reports what the test minima see: production and test
// lines are kept apart, tests are counted per type, and every exemption is
// printed with its reason so the debt stays visible.
func printTestInventory(cfg config, pkgs map[string]*packageReport) {
	if cfg.TestPolicy.MinTestFunctions == 0 && cfg.TestPolicy.MinBehaviorTests == 0 && cfg.TestPolicy.MinTestLOCRatio == 0 {
		return
	}
	prodLOC, testLOC, tests, behavior, fuzz, interop, testFiles := 0, 0, 0, 0, 0, 0, 0
	type shortfall struct {
		name    string
		missing string
	}
	var short []shortfall
	for _, pkg := range pkgs {
		prodLOC += pkg.LOC
		testLOC += pkg.TestLOC
		tests += pkg.TestFunctions
		behavior += pkg.BehaviorTests
		fuzz += pkg.FuzzTargets
		interop += pkg.InteropTests
		testFiles += pkg.TestFiles
		if pkg.LOC == 0 {
			continue
		}
		if _, ok := cfg.TestPolicy.Exemptions[pkg.Name]; ok {
			continue
		}
		var missing []string
		if cfg.TestPolicy.MinTestFunctions > 0 && pkg.TestFunctions < cfg.TestPolicy.MinTestFunctions {
			missing = append(missing, fmt.Sprintf("tests %d<%d", pkg.TestFunctions, cfg.TestPolicy.MinTestFunctions))
		}
		if cfg.TestPolicy.MinBehaviorTests > 0 && pkg.LOC >= cfg.TestPolicy.BehaviorMinProdLOC && pkg.BehaviorTests < cfg.TestPolicy.MinBehaviorTests {
			missing = append(missing, fmt.Sprintf("behaviour %d<%d", pkg.BehaviorTests, cfg.TestPolicy.MinBehaviorTests))
		}
		if len(missing) > 0 {
			short = append(short, shortfall{name: trimPackageName(pkg.Name), missing: strings.Join(missing, " ")})
		}
	}
	sort.Slice(short, func(i, j int) bool { return short[i].name < short[j].name })

	fmt.Println("Test Inventory:")
	ratio := 0.0
	if prodLOC > 0 {
		ratio = float64(testLOC) / float64(prodLOC)
	}
	fmt.Printf("- production_loc=%d test_loc=%d ratio=%.2f test_files=%d\n", prodLOC, testLOC, ratio, testFiles)
	fmt.Printf("- types: unit+other=%d behaviour=%d fuzz=%d interop=%d\n", tests-behavior-fuzz, behavior, fuzz, interop)
	if len(short) > 0 {
		fmt.Printf("- below the policy minimum: %d (each sits on a recorded baseline)\n", len(short))
		for _, item := range short {
			fmt.Printf("    %s (%s)\n", item.name, item.missing)
		}
	} else {
		fmt.Println("- below the policy minimum: 0")
	}
	onBaseline := 0
	for _, pkg := range pkgs {
		if _, ok := cfg.TestPolicy.BaselineTestFunctions[pkg.Name]; ok {
			onBaseline++
			continue
		}
		if _, ok := cfg.TestPolicy.BaselineBehaviorTests[pkg.Name]; ok {
			onBaseline++
			continue
		}
		if _, ok := cfg.TestPolicy.BaselineTestLOCRatio[pkg.Name]; ok {
			onBaseline++
		}
	}
	fmt.Printf("- policy minimum: tests>=%d behaviour>=%d (packages >=%d prod loc) ratio>=%.2f\n",
		cfg.TestPolicy.MinTestFunctions, cfg.TestPolicy.MinBehaviorTests, cfg.TestPolicy.BehaviorMinProdLOC, cfg.TestPolicy.MinTestLOCRatio)
	fmt.Printf("- on a baseline: %d packages\n", onBaseline)
	if len(cfg.TestPolicy.Exemptions) > 0 {
		names := make([]string, 0, len(cfg.TestPolicy.Exemptions))
		for name := range cfg.TestPolicy.Exemptions {
			names = append(names, name)
		}
		sort.Strings(names)
		fmt.Printf("- exemptions: %d\n", len(names))
		for _, name := range names {
			fmt.Printf("    %s: %s\n", trimPackageName(name), cfg.TestPolicy.Exemptions[name])
		}
	}
	fmt.Println()
}

func printReport(cfg config, files []fileReport, funcs []funcReport, pkgs map[string]*packageReport, failures, warnings []string) {
	fmt.Println("Matrix Code Governance Report")
	fmt.Println()
	fmt.Printf("Budgets: package=%d file=%d function=%d params=%d\n", cfg.Budgets.PackageLOC, cfg.Budgets.FileLOC, cfg.Budgets.FunctionLOC, cfg.Budgets.FunctionParams)
	fmt.Printf("Quality warnings: avg_func_loc<=%.2f p95_func_loc<=%.2f branch_points<=%d long_function_ratio<=%.2f\n", cfg.Quality.AvgFunctionLOCWarn, cfg.Quality.P95FunctionLOCWarn, cfg.Quality.BranchPointsWarn, cfg.Quality.LongFunctionRatio)
	if cfg.WarningBudget.MaxTotalWarnings > 0 || cfg.WarningBudget.MaxBranchPoints > 0 {
		fmt.Printf("Warning budget: total<=%d max_branch_points<=%d\n", cfg.WarningBudget.MaxTotalWarnings, cfg.WarningBudget.MaxBranchPoints)
	}
	fmt.Println()

	printTestInventory(cfg, pkgs)

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
		fmt.Printf("- %s lines=%d\n", file.Path, file.LOC)
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

func ensureTestBaselineInt(target *map[string]int, key, val string) error {
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

func ensureTestBaselineFloat(target *map[string]float64, key, val string) error {
	if *target == nil {
		*target = map[string]float64{}
	}
	f, err := atof(val)
	if err != nil {
		return err
	}
	(*target)[strings.Trim(key, `"`)] = f
	return nil
}

func parseString(raw string) (string, error) {
	return strings.Trim(strings.TrimSpace(raw), `"`), nil
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
