package agentmgr

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/middleware"
)

// sha256HexLength is the length of a hexadecimal encoded SHA-256 digest.
const sha256HexLength = 64

// verifyArtifact hashes the artifact Matrix just downloaded and compares it with
// the digest the registry index publishes for the current platform.
//
// It is the last gate before anything reaches the agent directory: extraction
// only starts when the bytes match, so a rejected artifact leaves no partial
// installation behind (the temporary download is removed by the caller).
//
// Three outcomes, and they stay distinguishable:
//
//   - the published digest matches: the install proceeds, verified=true;
//   - the index publishes no digest for this platform: the install proceeds,
//     but the evidence says "digest_not_published", never "verified";
//   - the digest differs, or the published value is not a valid sha256: the
//     install is refused with the reason.
func (inst *Installer) verifyArtifact(agentID, tmpFile, platform string, dist *BinaryDist) (*agentcfg.ArtifactVerification, error) {
	actual, err := hashFile(inst.fs, tmpFile)
	if err != nil {
		return nil, fmt.Errorf("cannot hash the downloaded artifact for %q: %w", agentID, err)
	}

	evidence := &agentcfg.ArtifactVerification{
		Status:     agentcfg.ArtifactDigestNotPublished,
		Platform:   platform,
		Artifact:   dist.Archive,
		Actual:     actual,
		VerifiedAt: time.Now().UTC(),
	}

	published := strings.ToLower(strings.TrimSpace(dist.SHA256))
	if published == "" {
		fmt.Printf("Registry index publishes no sha256 for %s (%s): installing without integrity verification\n", agentID, platform)
		return evidence, nil
	}
	if !isHexSHA256(published) {
		return nil, fmt.Errorf("artifact integrity check failed for %q: the registry index publishes a malformed sha256 %q for %s; refusing to install", agentID, dist.SHA256, dist.Archive)
	}
	if actual != published {
		return nil, fmt.Errorf("artifact integrity check failed for %q: sha256 mismatch for %s, the registry index publishes %s but the downloaded artifact has %s; refusing to install", agentID, dist.Archive, published, actual)
	}

	evidence.Verified = true
	evidence.Status = agentcfg.ArtifactVerified
	evidence.Expected = published
	return evidence, nil
}

// artifactVerificationNotApplicable is the evidence recorded when the resolved
// distribution downloads no artifact to hash (npx/uvx): nothing was verified,
// and the status says why.
func artifactVerificationNotApplicable() *agentcfg.ArtifactVerification {
	return &agentcfg.ArtifactVerification{
		Status:     agentcfg.ArtifactNotApplicable,
		VerifiedAt: time.Now().UTC(),
	}
}

// hashFile returns the lowercase hexadecimal sha256 of the file at path.
func hashFile(fs middleware.FS, path string) (string, error) {
	file, err := fs.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()

	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func isHexSHA256(value string) bool {
	if len(value) != sha256HexLength {
		return false
	}
	for _, char := range value {
		switch {
		case char >= '0' && char <= '9':
		case char >= 'a' && char <= 'f':
		default:
			return false
		}
	}
	return true
}
