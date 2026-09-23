package agentinstall

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// The ACP registry index is untrusted input. Two fields it publishes become
// part of the launch configuration Matrix persists and then executes: the
// binary `cmd` of a binary distribution, and the package identifier of an
// npx/uvx distribution. Both are checked here, where the index data turns into
// a path or an argv element, so a hostile entry is refused before anything is
// downloaded, extracted or registered.

// maxLauncherPathLength bounds the launcher path the index may publish. Every
// entry in the live index is far below it; the limit keeps a runaway value out
// of argv and out of the error, progress and doctor lines that echo it.
const maxLauncherPathLength = 256

// maxPackageSpecLength bounds the package identifier the index may publish. npm
// caps a package name at 214 characters, and 256 leaves room for a version
// suffix without letting an unbounded value through.
const maxPackageSpecLength = 256

// launcherPathCharacters is the character set a registry launcher path may use:
// exactly what every binary entry in the live ACP index uses (letters, digits,
// ".", "_", "+", "-", "/") plus "\", which Windows entries use as a separator.
// Shell metacharacters, whitespace, "~" and ":" are not in the set: no live
// entry needs them, and a launcher path is echoed into logs and doctor output.
const launcherPathCharacters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._+-/\\"

// npxPackageSpec is the shape npm accepts for the index's `npx.package`: an
// optional @scope/ prefix, a name, and an optional @version. It starts with a
// name character, so an option ("-c", "--registry") can never match.
var npxPackageSpec = regexp.MustCompile(`^(?:@[A-Za-z0-9_~][A-Za-z0-9._~-]*/)?[A-Za-z0-9_~][A-Za-z0-9._~-]*(?:@[A-Za-z0-9._+~^*<>=|-]+)?$`)

// uvxPackageSpec is the shape uv accepts for the index's `uvx.package`: a PEP
// 508 style requirement of a name, optional [extras] and an optional version
// specifier ("==", ">=", "~=", or uv's "@"). As above, it cannot start with an
// option.
var uvxPackageSpec = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*(?:\[[A-Za-z0-9._,-]+\])?(?:(?:===|==|~=|!=|<=|>=|<|>|@)[A-Za-z0-9._+*!-]+(?:,(?:===|==|!=|~=|<=|>=|<|>)[A-Za-z0-9._+*!-]+)*)?$`)

// validateLauncherPath reports why the cmd a binary distribution publishes
// cannot be launched from inside the agent directory. It is lexical only: the
// caller still has to prove the resolved file exists after extraction.
//
// It rejects an empty cmd, surrounding whitespace, an over-long value, an
// absolute path, a Windows volume or device name, anything containing ".."
// after cleaning, and any character outside launcherPathCharacters.
func validateLauncherPath(cmd string) error {
	if strings.TrimSpace(cmd) == "" {
		return fmt.Errorf("the registry index publishes an empty cmd for this platform")
	}
	if cmd != strings.TrimSpace(cmd) {
		return fmt.Errorf("the registry index publishes cmd %q with surrounding whitespace", cmd)
	}
	if len(cmd) > maxLauncherPathLength {
		return fmt.Errorf("the registry index publishes a %d-character cmd, over the %d allowed", len(cmd), maxLauncherPathLength)
	}
	if filepath.IsAbs(cmd) {
		return fmt.Errorf("the registry index publishes cmd %q as an absolute path; a launcher must live inside the agent directory", cmd)
	}
	if !filepath.IsLocal(cmd) {
		return fmt.Errorf("the registry index publishes cmd %q, which does not stay inside the agent directory", cmd)
	}
	for _, char := range cmd {
		if !strings.ContainsRune(launcherPathCharacters, char) {
			return fmt.Errorf("the registry index publishes cmd %q containing %q, which is not allowed in a launcher path", cmd, char)
		}
	}
	return nil
}

// ResolveLauncherPath validates cmd and resolves it inside agentDir. The result
// is guaranteed to be a descendant of agentDir: an absolute path, a ".."
// traversal or a Windows device name is refused before the download starts, so
// a hostile index entry costs no bytes and leaves nothing behind.
func ResolveLauncherPath(agentDir, cmd string) (string, error) {
	if err := validateLauncherPath(cmd); err != nil {
		return "", err
	}
	resolved := filepath.Join(agentDir, cmd)
	relative, err := filepath.Rel(agentDir, resolved)
	if err != nil || relative == ".." || filepath.IsAbs(relative) ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("the registry index publishes cmd %q, which escapes the agent directory %s", cmd, agentDir)
	}
	return resolved, nil
}

// ValidatePackageSpec reports why the package identifier an npx or uvx
// distribution publishes cannot be passed to npx/uvx. A leading "-" is refused
// outright: npx and uvx parse their own options anywhere on the command line,
// so an index value like "-c" or "--registry" would stop being a package and
// become an option that decides what actually runs.
func ValidatePackageSpec(kind, pkg string) error {
	if pkg == "" || pkg != strings.TrimSpace(pkg) {
		return fmt.Errorf("the registry index publishes an empty or padded %s package identifier %q", kind, pkg)
	}
	if len(pkg) > maxPackageSpecLength {
		return fmt.Errorf("the registry index publishes a %d-character %s package identifier, over the %d allowed", len(pkg), kind, maxPackageSpecLength)
	}
	if strings.HasPrefix(pkg, "-") {
		return fmt.Errorf("the registry index publishes %s package identifier %q, which npx/uvx would read as an option", kind, pkg)
	}
	var spec *regexp.Regexp
	switch kind {
	case "npx":
		spec = npxPackageSpec
	case "uvx":
		spec = uvxPackageSpec
	default:
		return fmt.Errorf("unsupported distribution type %q", kind)
	}
	if !spec.MatchString(pkg) {
		return fmt.Errorf("the registry index publishes %s package identifier %q, which is not a valid package name", kind, pkg)
	}
	return nil
}
