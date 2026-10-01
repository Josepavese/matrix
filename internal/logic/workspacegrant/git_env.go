package workspacegrant

import "github.com/Josepavese/matrix/internal/logic/childenv"

// gitEnv is the environment of the repository probe: the shared child allowlist,
// and nothing else.
//
// Git reads several variables that choose what it inspects: GIT_DIR and
// GIT_WORK_TREE redirect the repository, GIT_COMMON_DIR redirects the common
// directory this package keys a grant by, GIT_CEILING_DIRECTORIES and
// GIT_DISCOVERY_ACROSS_FILESYSTEM change how a repository is discovered, and
// GIT_CONFIG_GLOBAL and GIT_CONFIG_SYSTEM change which configuration it reads.
// None of them is on the shared list, deliberately: git answers correctly in a
// normal repository without any of them, so inheriting one could only ever move
// the verdict this probe decides.
func gitEnv() []string { return childenv.Environment() }
