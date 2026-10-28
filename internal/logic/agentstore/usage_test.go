package agentstore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// publishThreeGenerations publishes three distinct generations into store and
// returns their digests in publication order.
func publishThreeGenerations(t *testing.T, store *Store) []string {
	t.Helper()
	digests := make([]string, 0, 3)
	for _, body := range []string{"#!/bin/sh\necho one\n", "#!/bin/sh\necho two\n", "#!/bin/sh\necho three\n"} {
		_, digest, _ := publishTarGz(t, store, []archiveEntry{
			{name: "opencode", mode: 0o755, body: body},
			{name: "lib/data.txt", mode: 0o644, body: "artifact data"},
		})
		digests = append(digests, digest)
	}
	return digests
}

// referenceHome records every digest under a distinct agent id in home, so one
// home can hold more than one reference.
func referenceHome(t *testing.T, home string, digests []string) {
	t.Helper()
	for index, digest := range digests {
		ref := Reference{Digest: digest, Launcher: "opencode", Platform: "linux/amd64"}
		if err := WriteReference(home, "agent-"+digest[:8]+string(rune('a'+index)), ref); err != nil {
			t.Fatalf("WriteReference: %v", err)
		}
	}
}

func measuredBytes(usage Usage) map[string]int64 {
	bytes := make(map[string]int64, len(usage.Generations))
	for _, generation := range usage.Generations {
		bytes[generation.Digest] = generation.Bytes
	}
	return bytes
}

func TestMeasureClassifiesGenerationBytesByReferenceFanout(t *testing.T) {
	store := newTestPublisher(t)
	digests := publishThreeGenerations(t, store)
	if len(digests) != 3 {
		t.Fatalf("published %d generations, want 3", len(digests))
	}
	homeOne, homeTwo := t.TempDir(), t.TempDir()
	// Only the first home needs the first generation; both homes need the second;
	// nothing needs the third.
	referenceHome(t, homeOne, []string{digests[0], digests[1]})
	referenceHome(t, homeTwo, []string{digests[1]})

	fanout, err := LoadRefFanout([]string{homeOne, homeTwo})
	if err != nil {
		t.Fatalf("LoadRefFanout: %v", err)
	}
	usage, err := store.Measure(fanout)
	if err != nil {
		t.Fatalf("Measure: %v", err)
	}

	if len(usage.Generations) != 3 {
		t.Fatalf("measured %d generations, want 3", len(usage.Generations))
	}
	bytes := measuredBytes(usage)
	for _, digest := range digests {
		if bytes[digest] <= 0 {
			t.Fatalf("generation %s measured %d bytes, want a real size", digest, bytes[digest])
		}
	}
	if usage.ExclusiveBytes != bytes[digests[0]] {
		t.Fatalf("exclusive bytes = %d, want %d (the generation one home needs)", usage.ExclusiveBytes, bytes[digests[0]])
	}
	if usage.SharedBytes != bytes[digests[1]] {
		t.Fatalf("shared bytes = %d, want %d (the generation two homes need)", usage.SharedBytes, bytes[digests[1]])
	}
	if usage.ReclaimableBytes != bytes[digests[2]] {
		t.Fatalf("reclaimable bytes = %d, want %d (the generation no home needs)", usage.ReclaimableBytes, bytes[digests[2]])
	}
	if usage.TotalBytes != bytes[digests[0]]+bytes[digests[1]]+bytes[digests[2]] {
		t.Fatalf("total bytes = %d, want the sum of the three generations", usage.TotalBytes)
	}
	if usage.ExclusiveBytes+usage.SharedBytes+usage.ReclaimableBytes != usage.TotalBytes {
		t.Fatalf("the buckets do not add up to the total: %d+%d+%d != %d",
			usage.ExclusiveBytes, usage.SharedBytes, usage.ReclaimableBytes, usage.TotalBytes)
	}
}

func TestMeasureCountsPublishStagingApartFromPublishedSoftware(t *testing.T) {
	store := newTestPublisher(t)
	publishThreeGenerations(t, store)
	staging := filepath.Join(store.tmpDir(), "publish-abandoned")
	writeFile(t, filepath.Join(staging, "partial"), "half a tree", 0o644)

	usage, err := store.Measure(map[string]int{})
	if err != nil {
		t.Fatalf("Measure: %v", err)
	}
	if usage.StagingBytes != int64(len("half a tree")) {
		t.Fatalf("staging bytes = %d, want %d", usage.StagingBytes, len("half a tree"))
	}
	for _, generation := range usage.Generations {
		if generation.Bytes == usage.StagingBytes {
			t.Fatalf("staging was counted as published software of %s", generation.Digest)
		}
	}
	if usage.ReclaimableBytes != usage.TotalBytes {
		t.Fatalf("with no home given every generation must be reclaimable: reclaimable %d, total %d", usage.ReclaimableBytes, usage.TotalBytes)
	}
}

func TestLoadRefFanoutCountsEveryHomeThatSharesADigest(t *testing.T) {
	digest := hexDigest("a")
	homes := []string{t.TempDir(), t.TempDir(), t.TempDir()}
	referenceHome(t, homes[0], []string{digest})
	referenceHome(t, homes[1], []string{digest})

	fanout, err := LoadRefFanout(homes)
	if err != nil {
		t.Fatalf("LoadRefFanout: %v", err)
	}
	if fanout[digest] != 2 {
		t.Fatalf("fanout for %s = %d, want 2", digest, fanout[digest])
	}
	if len(fanout) != 1 {
		t.Fatalf("fanout carries %d digests, want 1", len(fanout))
	}

	unreadable := []string{t.TempDir()}
	if err := os.MkdirAll(filepath.Join(unreadable[0], "configs"), 0o700); err != nil {
		t.Fatalf("create reference directory: %v", err)
	}
	writeFile(t, ReferencePath(unreadable[0]), "{not json", 0o600)
	if _, err := LoadRefFanout(unreadable); err == nil {
		t.Fatal("LoadRefFanout must refuse a malformed reference document instead of counting it as no references")
	}
}

func TestOpenInspectionReadsAStoreWithoutClaimingTheConsumerRole(t *testing.T) {
	store := newTestPublisher(t)
	digests := publishThreeGenerations(t, store)

	_, consumerErr := OpenConsumer(store.root, "")
	if !errors.Is(consumerErr, ErrLayoutWritable) {
		t.Fatalf("OpenConsumer on a writable layout = %v, want ErrLayoutWritable", consumerErr)
	}

	inspection, err := OpenInspection(store.root, "")
	if err != nil {
		t.Fatalf("OpenInspection: %v", err)
	}
	if err := inspection.Verify(digests[0]); err != nil {
		t.Fatalf("a generation of a writable store must still be measurable: %v", err)
	}
	if _, _, err := inspection.GC(nil); err == nil {
		t.Fatal("an inspection store must refuse to mutate the store")
	}

	sealStoreForConsumer(t, store)
	consumer, err := OpenConsumer(store.root, "")
	if err != nil {
		t.Fatalf("OpenConsumer on a sealed layout: %v", err)
	}
	if err := consumer.Verify(digests[0]); err != nil {
		t.Fatalf("a sealed store must verify its generations: %v", err)
	}
}
