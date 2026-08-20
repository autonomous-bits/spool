package repository

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/fxamacker/cbor/v2"
	"github.com/pelletier/go-toml/v2"
)

// ErrFsckCorrupt reports that Fsck found one or more integrity violations.
var ErrFsckCorrupt = errors.New("repository integrity check failed")

// FsckDiagnostic describes one deterministic integrity or maintenance finding.
type FsckDiagnostic struct {
	Code   string   `json:"code"`
	Path   string   `json:"path,omitempty"`
	Branch string   `json:"branch,omitempty"`
	Object ObjectID `json:"object,omitempty"`
	Detail string   `json:"detail"`
}

// FsckResult is the complete report produced by an integrity check.
type FsckResult struct {
	Valid         bool             `json:"valid"`
	Branches      []string         `json:"branches"`
	Commits       int              `json:"commits"`
	Snapshots     int              `json:"snapshots"`
	Objects       int              `json:"objects"`
	Diagnostics   []FsckDiagnostic `json:"diagnostics"`
	Informational []FsckDiagnostic `json:"informational,omitempty"`
}

// FsckError carries the structured report for a corrupt repository.
type FsckError struct {
	Result FsckResult
}

func (e *FsckError) Error() string {
	return fmt.Sprintf("%s: %d diagnostic(s)", ErrFsckCorrupt, len(e.Result.Diagnostics))
}

func (e *FsckError) Unwrap() error { return ErrFsckCorrupt }

// FsckRepository traverses every durable branch reference and checks the
// reachable immutable objects and mutable control state without repairing it.
func FsckRepository(stateDir string) (FsckResult, error) {
	checker := newFsckChecker(stateDir)
	checker.checkPacks()
	checker.checkControlState()
	checker.checkReflogs()
	checker.checkStagedState()
	checker.checkMergeTransactions()
	checker.checkLooseObjects()
	return checker.result()
}

// Fsck checks an opened repository's durable state when it has one, or its
// current immutable in-memory graph otherwise.
func (r *Repository) Fsck() (FsckResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if err := r.ensureOpenLocked(); err != nil {
		return FsckResult{}, err
	}
	if r.mergeStateDir != "" {
		return FsckRepository(r.mergeStateDir)
	}

	checker := newFsckChecker("")
	checker.branches = cloneBranches(r.branches)
	checker.objects = cloneObjects(r.objects)
	checker.staged = cloneStagedMutations(r.stagedMutations)
	for _, branch := range sortedFsckBranches(checker.branches) {
		checker.checkCommit(checker.branches[branch], branch, make(map[ObjectID]bool))
	}
	checker.checkStagedState()
	return checker.result()
}

type fsckChecker struct {
	stateDir        string
	branches        map[string]ObjectID
	commits         map[ObjectID]commit
	snapshots       map[ObjectID]graphSnapshot
	nodes           map[ObjectID]map[string]Node
	edges           map[ObjectID]map[string]Edge
	objects         map[ObjectID][]byte
	staged          map[string]StagedMutationSet
	snapshotValid   map[ObjectID]bool
	seen            map[ObjectID]struct{}
	reachable       map[ObjectID]struct{}
	packed          map[ObjectID]fsckStoredObject
	loose           map[ObjectID]fsckLooseObject
	locationChecked map[ObjectID]struct{}
	issues          []FsckDiagnostic
	informational   []FsckDiagnostic
}

type fsckStoredObject struct {
	objectType string
	data       []byte
	path       string
}

type fsckLooseObject struct {
	fsckStoredObject
	present bool
	valid   bool
}

func newFsckChecker(stateDir string) *fsckChecker {
	return &fsckChecker{
		stateDir: stateDir, branches: make(map[string]ObjectID), commits: make(map[ObjectID]commit),
		snapshots: make(map[ObjectID]graphSnapshot), nodes: make(map[ObjectID]map[string]Node),
		edges: make(map[ObjectID]map[string]Edge), objects: make(map[ObjectID][]byte),
		staged: make(map[string]StagedMutationSet), snapshotValid: make(map[ObjectID]bool),
		seen: make(map[ObjectID]struct{}), reachable: make(map[ObjectID]struct{}),
		packed: make(map[ObjectID]fsckStoredObject), loose: make(map[ObjectID]fsckLooseObject),
		locationChecked: make(map[ObjectID]struct{}),
	}
}

func (c *fsckChecker) result() (FsckResult, error) {
	sortFsckDiagnostics(c.issues)
	sortFsckDiagnostics(c.informational)
	result := FsckResult{
		Valid: len(c.issues) == 0, Branches: sortedFsckBranches(c.branches),
		Commits: len(c.commits), Snapshots: len(c.snapshots), Objects: len(c.seen),
		Diagnostics: c.issues, Informational: c.informational,
	}
	if result.Diagnostics == nil {
		result.Diagnostics = []FsckDiagnostic{}
	}
	if !result.Valid {
		return result, &FsckError{Result: result}
	}
	return result, nil
}

func sortFsckDiagnostics(diagnostics []FsckDiagnostic) {
	sort.Slice(diagnostics, func(i, j int) bool {
		left, right := diagnostics[i], diagnostics[j]
		if left.Code != right.Code {
			return left.Code < right.Code
		}
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		if left.Branch != right.Branch {
			return left.Branch < right.Branch
		}
		if left.Object != right.Object {
			return left.Object < right.Object
		}
		return left.Detail < right.Detail
	})
}

func (c *fsckChecker) issue(code, path, branch string, object ObjectID, detail string) {
	c.issues = append(c.issues, FsckDiagnostic{
		Code: code, Path: filepath.ToSlash(path), Branch: branch, Object: object, Detail: detail,
	})
}

func (c *fsckChecker) inform(code, path, branch string, object ObjectID, detail string) {
	c.informational = append(c.informational, FsckDiagnostic{
		Code: code, Path: filepath.ToSlash(path), Branch: branch, Object: object, Detail: detail,
	})
}

func (c *fsckChecker) checkControlState() {
	if c.stateDir == "" {
		return
	}
	configPath := filepath.Join(c.stateDir, "config.toml")
	configData, ok := c.controlFile(configPath, "config.toml")
	if !ok {
		return
	}
	var config repositoryConfig
	if err := toml.Unmarshal(configData, &config); err != nil || config.FormatVersion != repositoryFormatVersion || !validRefName(config.DefaultBranch) {
		c.issue("invalid-config", "config.toml", "", "", "configuration is not a supported repository configuration")
		return
	}
	head, headOK := c.controlValue(filepath.Join(c.stateDir, "HEAD"), "HEAD")
	if !headOK || !validRefName(head) {
		c.issue("invalid-head", "HEAD", "", "", "HEAD does not name a valid branch")
	}
	c.branches = c.readRefs()
	if len(c.branches) == 0 {
		c.issue("missing-refs", "refs/heads", "", "", "repository has no branch references")
	}
	if c.branches[config.DefaultBranch] == "" {
		c.issue("invalid-default-branch", "config.toml", config.DefaultBranch, "", "default branch has no reference")
	}
	if headOK && c.branches[head] == "" {
		c.issue("invalid-head", "HEAD", head, "", "HEAD branch has no reference")
	}
	for _, branch := range sortedFsckBranches(c.branches) {
		c.checkCommit(c.branches[branch], branch, make(map[ObjectID]bool))
	}
}

func (c *fsckChecker) controlFile(path, relative string) ([]byte, bool) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		c.issue("missing-control-file", relative, "", "", "file is missing")
		return nil, false
	}
	if err != nil {
		c.issue("read-control-file", relative, "", "", "cannot inspect file")
		return nil, false
	}
	if !info.Mode().IsRegular() {
		c.issue("invalid-control-file", relative, "", "", "file is not regular")
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		c.issue("read-control-file", relative, "", "", "cannot read file")
		return nil, false
	}
	return data, true
}

func (c *fsckChecker) controlValue(path, relative string) (string, bool) {
	data, ok := c.controlFile(path, relative)
	if !ok {
		return "", false
	}
	value := strings.TrimSuffix(string(data), "\n")
	if value == "" || strings.ContainsAny(value, "\r\n") {
		c.issue("invalid-control-value", relative, "", "", "value must be one non-empty line")
		return "", false
	}
	return value, true
}

func (c *fsckChecker) readRefs() map[string]ObjectID {
	refs := make(map[string]ObjectID)
	root := filepath.Join(c.stateDir, "refs", "heads")
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			c.issue("read-ref", filepath.ToSlash(strings.TrimPrefix(path, c.stateDir+string(filepath.Separator))), "", "", "cannot walk branch references")
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			c.issue("invalid-ref", "", "", "", "cannot determine reference name")
			return nil
		}
		name := filepath.ToSlash(relative)
		controlPath := filepath.ToSlash(filepath.Join("refs", "heads", name))
		if !entry.Type().IsRegular() || !validRefName(name) {
			c.issue("invalid-ref", controlPath, name, "", "reference must be a regular file with a valid name")
			return nil
		}
		value, ok := c.controlValue(path, controlPath)
		if !ok || !validLooseObjectID(ObjectID(value)) {
			c.issue("invalid-ref", controlPath, name, "", "reference must contain an object ID")
			return nil
		}
		refs[name] = ObjectID(value)
		return nil
	})
	if os.IsNotExist(err) {
		c.issue("missing-refs", "refs/heads", "", "", "branch reference directory is missing")
	} else if err != nil {
		c.issue("read-ref", "refs/heads", "", "", "cannot read branch references")
	}
	return refs
}

// checkReflogs validates the durable retention inventory before treating a
// reflog as a GC root. A reflog on disk alone never expands retention.
func (c *fsckChecker) checkReflogs() {
	if c.stateDir == "" {
		return
	}
	root := filepath.Join(c.stateDir, "logs")
	inventoryRelative := reflogRetentionInventoryFilename
	paths, inventoryErr := readReflogRetentionInventory(filepath.Join(c.stateDir, inventoryRelative))
	if errors.Is(inventoryErr, errReflogRetentionInventoryMissing) {
		c.issue("missing-reflog-retention-inventory", inventoryRelative, "", "", "reflog retention inventory is missing")
	} else if inventoryErr != nil {
		c.issue("invalid-reflog-retention-inventory", inventoryRelative, "", "", "reflog retention inventory is malformed")
	}
	actual := c.discoverReflogs(root)
	if inventoryErr != nil {
		return
	}
	expected := reflogRetentionPathSet(paths)
	actualSet := reflogRetentionPathSet(actual)
	for _, path := range paths {
		if _, found := actualSet[path]; !found {
			c.issue("missing-listed-reflog", filepath.ToSlash(filepath.Join("logs", path)), "", "", "reflog listed by retention inventory is missing")
		}
	}
	for _, path := range actual {
		if _, found := expected[path]; !found {
			c.issue("unexpected-reflog", filepath.ToSlash(filepath.Join("logs", path)), "", "", "reflog is not listed by retention inventory")
		}
	}
	for _, relative := range paths {
		if _, found := actualSet[relative]; !found {
			continue
		}
		path := filepath.Join(root, filepath.FromSlash(relative))
		displayPath := filepath.ToSlash(filepath.Join("logs", relative))
		if relative == "HEAD" {
			c.checkHeadReflog(path, displayPath)
			continue
		}
		c.checkObjectReflog(path, displayPath, strings.TrimPrefix(relative, "refs/heads/"))
	}
}

func (c *fsckChecker) discoverReflogs(root string) []string {
	paths := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if path == root && os.IsNotExist(walkErr) {
				return walkErr
			}
			c.issue("read-reflog", "logs", "", "", "cannot walk reflogs")
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			c.issue("invalid-reflog-path", "logs", "", "", "cannot determine reflog path")
			return nil
		}
		relative = filepath.ToSlash(relative)
		displayPath := filepath.ToSlash(filepath.Join("logs", relative))
		if !entry.Type().IsRegular() {
			c.issue("invalid-reflog-file", displayPath, "", "", "reflog must be a regular file")
			return nil
		}
		if _, err := canonicalReflogRetentionPath(relative); err != nil {
			c.issue("out-of-scope-reflog", displayPath, "", "", "reflog path is outside the supported retention namespace")
			return nil
		}
		paths = append(paths, relative)
		return nil
	})
	if os.IsNotExist(err) {
		c.issue("missing-reflog-directory", "logs", "", "", "reflog directory is missing")
	} else if err != nil {
		c.issue("read-reflog", "logs", "", "", "cannot read reflogs")
	}
	sort.Strings(paths)
	return paths
}

func (c *fsckChecker) checkHeadReflog(path, displayPath string) {
	lines, err := readReflogLines(path)
	if err != nil {
		c.issue("invalid-reflog-entry", displayPath, "", "", "reflog entries are malformed")
		return
	}
	for _, fields := range lines {
		for _, value := range fields[:2] {
			if value != "" && !validRefName(value) {
				c.issue("invalid-head-reflog-reference", displayPath, "", "", "HEAD reflog contains an invalid branch reference")
			}
		}
	}
}

func (c *fsckChecker) checkObjectReflog(path, displayPath, branch string) {
	lines, err := readReflogLines(path)
	if err != nil {
		c.issue("invalid-reflog-entry", displayPath, branch, "", "reflog entries are malformed")
		return
	}
	for _, fields := range lines {
		for _, value := range fields[:2] {
			if value == "" {
				continue
			}
			id := ObjectID(value)
			if !validLooseObjectID(id) {
				c.issue("invalid-reflog-object-id", displayPath, branch, id, "reflog contains an invalid object ID")
				continue
			}
			c.checkCommit(id, branch, make(map[ObjectID]bool))
		}
	}
}

func (c *fsckChecker) object(id ObjectID, expectedType string) ([]byte, bool) {
	data, _, ok := c.objectAny(id, []string{expectedType})
	return data, ok
}

func (c *fsckChecker) objectAny(id ObjectID, expectedTypes []string) ([]byte, string, bool) {
	if !validLooseObjectID(id) {
		c.issue("invalid-object-id", "", "", id, "object ID is not lowercase hexadecimal")
		return nil, "", false
	}
	c.seen[id] = struct{}{}
	c.reachable[id] = struct{}{}
	if c.stateDir == "" {
		data, ok := c.objects[id]
		if !ok {
			c.issue("missing-object", "", "", id, "object is absent from the object store")
			return nil, "", false
		}
		for _, expectedType := range expectedTypes {
			if objectIDForEncoded(expectedType, data) == id {
				return append([]byte(nil), data...), expectedType, true
			}
		}
		if len(expectedTypes) == 0 {
			c.issue("invalid-object-type", "", "", id, "in-memory object type cannot be determined")
			return nil, "", false
		}
		c.issue("object-type-mismatch", "", "", id, fmt.Sprintf("object type is not one of %q", expectedTypes))
		return nil, "", false
	}
	loose := c.readLooseObject(id)
	packed, packedOK := c.packed[id]
	c.checkDuplicateLocation(id, loose, packed, packedOK)
	var object fsckStoredObject
	switch {
	case loose.valid:
		object = loose.fsckStoredObject
	case packedOK:
		object = packed
	default:
		if !loose.present {
			c.issue("missing-object", loose.path, "", id, "object has no loose or active packed location")
		}
		return nil, "", false
	}
	if len(expectedTypes) == 0 {
		if !knownLooseObjectType(object.objectType) {
			c.issue("invalid-object-type", object.path, "", id, "object envelope has an unsupported type")
			return nil, "", false
		}
		c.objects[id] = append([]byte(nil), object.data...)
		return object.data, object.objectType, true
	}
	for _, expectedType := range expectedTypes {
		if object.objectType == expectedType {
			c.objects[id] = append([]byte(nil), object.data...)
			return object.data, object.objectType, true
		}

	}
	c.issue("object-type-mismatch", object.path, "", id, fmt.Sprintf("object type is %q, want one of %q", object.objectType, expectedTypes))
	return nil, "", false
}

func knownLooseObjectType(objectType string) bool {
	switch objectType {
	case "commit", "graph-snapshot", "schema-root", "node", "edge", prollyTreeLeafType, prollyTreeInternalType:
		return true
	default:
		return false
	}
}

func (c *fsckChecker) checkPacks() {
	if c.stateDir == "" {
		return
	}
	store := newLooseObjectStore(c.stateDir, &c.objects)
	manifest, err := store.readPackManifest()
	if err != nil {
		c.packIssue("objects/info/packs", "", err)
		return
	}
	for _, metadata := range manifest.Packs {
		packPath, err := store.packPath(metadata.ID)
		if err != nil {
			c.packIssue("objects/info/packs", metadata.ID, err)
			continue
		}
		index, err := store.openPackIndex(metadata)
		if err != nil {
			c.packIssue(filepath.ToSlash(filepath.Join("objects", "pack", string(metadata.ID)+packIndexFileExtension)), metadata.ID, err)
			continue
		}
		verifyErr := verifyPackFiles(store.packIndexes, metadata.ID, packPath, filepath.Join(store.packDirectory(), string(metadata.ID)+packIndexFileExtension))
		if verifyErr != nil {
			_ = index.Close()
			c.packIssue(filepath.ToSlash(filepath.Join("objects", "pack", string(metadata.ID)+packFileExtension)), metadata.ID, verifyErr)
			continue
		}
		err = index.ForEach(func(entry PackIndexEntry) error {
			objectType, data, readErr := readPackedObjectFile(metadata.ID, packPath, entry, metadata.ObjectCount)
			if readErr != nil {
				return readErr
			}
			c.seen[entry.Object] = struct{}{}
			object := fsckStoredObject{
				objectType: objectType, data: append([]byte(nil), data...),
				path: filepath.ToSlash(filepath.Join("objects", "pack", string(metadata.ID)+packFileExtension)),
			}
			if !knownLooseObjectType(objectType) {
				c.issue("invalid-object-type", object.path, "", entry.Object, "packed object envelope has an unsupported type")
			}
			if existing, duplicate := c.packed[entry.Object]; duplicate {
				if existing.objectType != object.objectType || !bytes.Equal(existing.data, object.data) {
					c.issue("inconsistent-object-location", object.path, "", entry.Object, "active packs have different types or payloads for the same object")
				}
				return nil
			}
			c.packed[entry.Object] = object
			c.objects[entry.Object] = append([]byte(nil), data...)
			return nil
		})
		closeErr := index.Close()
		if err != nil {
			c.packIssue(filepath.ToSlash(filepath.Join("objects", "pack", string(metadata.ID)+packFileExtension)), metadata.ID, err)
		}
		if closeErr != nil {
			c.packIssue(filepath.ToSlash(filepath.Join("objects", "pack", string(metadata.ID)+packIndexFileExtension)), metadata.ID, closeErr)
		}
	}
}

func (c *fsckChecker) packIssue(path string, pack PackID, err error) {
	code := "invalid-pack"
	if errors.Is(err, ErrUnsupportedPackVersion) {
		code = "unsupported-pack-version"
	}
	detail := err.Error()
	if pack != "" {
		detail = fmt.Sprintf("pack %q: %s", pack, detail)
	}
	c.issue(code, path, "", "", detail)
}

func (c *fsckChecker) checkLooseObjects() {
	if c.stateDir == "" {
		return
	}
	root := filepath.Join(c.stateDir, "objects", "loose")
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if path == root && os.IsNotExist(walkErr) {
				c.issue("missing-loose-objects", "objects/loose", "", "", "loose object directory is missing")
				return nil
			}
			c.issue("read-object", "objects/loose", "", "", "cannot walk loose objects")
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			c.issue("invalid-object-path", "objects/loose", "", "", "cannot determine object ID")
			return nil
		}
		id := ObjectID(strings.ReplaceAll(filepath.ToSlash(relative), "/", ""))
		displayPath := filepath.ToSlash(filepath.Join("objects", "loose", relative))
		if !entry.Type().IsRegular() || !validLooseObjectID(id) ||
			filepath.ToSlash(relative) != filepath.ToSlash(filepath.Join(string(id[:2]), string(id[2:]))) {
			c.issue("invalid-object-path", displayPath, "", "", "loose object path does not match an object ID")
			return nil
		}
		c.seen[id] = struct{}{}
		loose := c.readLooseObject(id)
		packed, packedOK := c.packed[id]
		c.checkDuplicateLocation(id, loose, packed, packedOK)
		if !loose.valid {
			return nil
		}
		if !knownLooseObjectType(loose.objectType) {
			c.issue("invalid-object-type", loose.path, "", id, "object envelope has an unsupported type")
			return nil
		}
		c.objects[id] = append([]byte(nil), loose.data...)
		if _, reachable := c.reachable[id]; !reachable {
			c.inform("unreachable-loose-object", loose.path, "", id, "unreachable loose object is a GC candidate subject to the retention grace period")
		}
		return nil
	})
	if os.IsNotExist(err) {
		c.issue("missing-loose-objects", "objects/loose", "", "", "loose object directory is missing")
	} else if err != nil {
		c.issue("read-object", "objects/loose", "", "", "cannot read loose objects")
	}
}

func (c *fsckChecker) readLooseObject(id ObjectID) fsckLooseObject {
	if object, checked := c.loose[id]; checked {
		return object
	}
	relative := filepath.ToSlash(filepath.Join("objects", "loose", string(id[:2]), string(id[2:])))
	path := filepath.Join(c.stateDir, filepath.FromSlash(relative))
	result := fsckLooseObject{fsckStoredObject: fsckStoredObject{path: relative}}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		c.loose[id] = result
		return result
	}
	result.present = true
	if err != nil {
		c.issue("read-object", relative, "", id, "cannot inspect object file")
		c.loose[id] = result
		return result
	}
	if !info.Mode().IsRegular() {
		c.issue("invalid-object-file", relative, "", id, "object file is not regular")
		c.loose[id] = result
		return result
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		c.issue("read-object", relative, "", id, "cannot read object file")
		c.loose[id] = result
		return result
	}
	var envelope looseObjectEnvelope
	if err := cbor.Unmarshal(raw, &envelope); err != nil {
		c.issue("invalid-object-envelope", relative, "", id, "object envelope cannot be decoded")
		c.loose[id] = result
		return result
	}
	canonical, err := canonicalCBOR.Marshal(envelope)
	if err != nil || !bytes.Equal(raw, canonical) {
		c.issue("noncanonical-object-envelope", relative, "", id, "object envelope is not canonical CBOR")
		c.loose[id] = result
		return result
	}
	if objectIDForEncoded(envelope.Type, envelope.Data) != id {
		c.issue("object-hash-mismatch", relative, "", id, "object payload hash does not match its ID")
		c.loose[id] = result
		return result
	}
	result.objectType = envelope.Type
	result.data = append([]byte(nil), envelope.Data...)
	result.valid = true
	c.loose[id] = result
	return result
}

func (c *fsckChecker) checkDuplicateLocation(id ObjectID, loose fsckLooseObject, packed fsckStoredObject, packedOK bool) {
	if !loose.valid || !packedOK {
		return
	}
	if _, checked := c.locationChecked[id]; checked {
		return
	}
	c.locationChecked[id] = struct{}{}
	if loose.objectType != packed.objectType || !bytes.Equal(loose.data, packed.data) {
		c.issue("inconsistent-object-location", loose.path, "", id, "loose and packed copies have different types or payloads")
	}
}

func (c *fsckChecker) decodeObject(id ObjectID, objectType string, target any) bool {
	data, ok := c.object(id, objectType)
	if !ok {
		return false
	}
	if err := cbor.Unmarshal(data, target); err != nil {
		c.issue("invalid-object-payload", "", "", id, "object payload cannot be decoded")
		return false
	}
	encoded, err := canonicalObjectEncoding(reflect.ValueOf(target).Elem().Interface())
	if err != nil || !bytes.Equal(data, encoded) {
		c.issue("noncanonical-object-payload", "", "", id, "object payload is not canonical CBOR")
		return false
	}
	return true
}

func (c *fsckChecker) checkCommit(id ObjectID, branch string, visiting map[ObjectID]bool) {
	if _, loaded := c.commits[id]; loaded {
		return
	}
	if visiting[id] {
		c.issue("commit-cycle", "", branch, id, "commit ancestry contains a cycle")
		return
	}
	visiting[id] = true
	defer delete(visiting, id)
	var value commit
	if !c.decodeObject(id, "commit", &value) {
		return
	}
	if value.Snapshot == "" {
		c.issue("invalid-commit", "", branch, id, "commit has no snapshot")
		return
	}
	parents := make(map[ObjectID]struct{}, len(value.Parents))
	for _, parent := range value.Parents {
		if !validLooseObjectID(parent) {
			c.issue("invalid-parent", "", branch, id, "commit parent is not an object ID")
			continue
		}
		if _, duplicate := parents[parent]; duplicate {
			c.issue("duplicate-parent", "", branch, id, "commit lists a parent more than once")
			continue
		}
		parents[parent] = struct{}{}
		c.checkCommit(parent, branch, visiting)
	}
	c.commits[id] = value
	c.checkSnapshot(value.Snapshot, branch)
}

func (c *fsckChecker) checkSnapshot(id ObjectID, branch string) (valid bool) {
	if checked, loaded := c.snapshotValid[id]; loaded {
		return checked
	}
	issueCount := len(c.issues)
	c.snapshotValid[id] = false
	defer func() {
		valid = len(c.issues) == issueCount
		c.snapshotValid[id] = valid
	}()
	var snapshot graphSnapshot
	if !c.decodeObject(id, "graph-snapshot", &snapshot) {
		return false
	}
	c.snapshots[id] = snapshot
	if snapshot.NodeRoot == "" || snapshot.EdgeRoot == "" || snapshot.OutAdjRoot == "" || snapshot.InAdjRoot == "" || snapshot.SchemaRoot == "" {
		c.issue("invalid-snapshot", "", branch, id, "snapshot has an empty root")
		return false
	}
	var schema SchemaSnapshot
	if c.decodeObject(snapshot.SchemaRoot, "schema-root", &schema) {
		normalized, err := schema.Normalize()
		if err != nil || !reflect.DeepEqual(schema, normalized) {
			c.issue("invalid-schema", "", branch, snapshot.SchemaRoot, "schema is not normalized")
		}
	}
	nodeEntries, nodeOK := c.checkProllyTree(snapshot.NodeRoot)
	edgeEntries, edgeOK := c.checkProllyTree(snapshot.EdgeRoot)
	outEntries, outOK := c.checkProllyTree(snapshot.OutAdjRoot)
	inEntries, inOK := c.checkProllyTree(snapshot.InAdjRoot)
	if !nodeOK || !edgeOK || !outOK || !inOK {
		return false
	}

	nodes := make(map[string]Node, len(nodeEntries))
	for _, entry := range nodeEntries {
		var node Node
		if !c.decodeObject(entry.Value, "node", &node) {
			continue
		}
		normalized, err := node.Normalize()
		if node.ID == "" || node.ID != entry.Key || err != nil || !reflect.DeepEqual(node, normalized) {
			c.issue("invalid-node", "", branch, entry.Value, "node does not match its tree key or canonical form")
			continue
		}
		if _, duplicate := nodes[node.ID]; duplicate {
			c.issue("duplicate-node", "", branch, entry.Value, "snapshot contains a duplicate node ID")
			continue
		}
		nodes[node.ID] = node
	}
	edges := make(map[string]Edge, len(edgeEntries))
	for _, entry := range edgeEntries {
		var edge Edge
		if !c.decodeObject(entry.Value, "edge", &edge) {
			continue
		}
		normalized, err := edge.Normalize()
		if edge.ID == "" || edge.ID != entry.Key || edge.Source == "" || edge.Target == "" || err != nil || !reflect.DeepEqual(edge, normalized) {
			c.issue("invalid-edge", "", branch, entry.Value, "edge does not match its tree key or canonical form")
			continue
		}
		if _, duplicate := edges[edge.ID]; duplicate {
			c.issue("duplicate-edge", "", branch, entry.Value, "snapshot contains a duplicate edge ID")
			continue
		}
		edges[edge.ID] = edge
		if _, exists := nodes[edge.Source]; !exists {
			c.issue("missing-edge-source", "", branch, entry.Value, "edge source is absent from the node tree")
		}
		if _, exists := nodes[edge.Target]; !exists {
			c.issue("missing-edge-target", "", branch, entry.Value, "edge target is absent from the node tree")
		}
	}
	if uint64(len(nodeEntries)) != snapshot.NodeCount || uint64(len(edgeEntries)) != snapshot.EdgeCount {
		c.issue("snapshot-count-mismatch", "", branch, id, "snapshot counts do not match projection tree entries")
	}
	expectedOut, expectedIn := make([]prollyTreeEntry, 0, len(edgeEntries)), make([]prollyTreeEntry, 0, len(edgeEntries))
	for _, entry := range edgeEntries {
		edge, ok := edges[entry.Key]
		if !ok {
			continue
		}
		expectedOut = append(expectedOut, prollyTreeEntry{Key: adjacencyKey(edge.Source, edge.ID), Value: entry.Value})
		expectedIn = append(expectedIn, prollyTreeEntry{Key: adjacencyKey(edge.Target, edge.ID), Value: entry.Value})
	}
	if !sameProllyEntries(sortedProllyEntries(expectedOut), outEntries) {
		c.issue("invalid-out-adjacency", "", branch, snapshot.OutAdjRoot, "outgoing adjacency does not match the edge tree")
	}
	if !sameProllyEntries(sortedProllyEntries(expectedIn), inEntries) {
		c.issue("invalid-in-adjacency", "", branch, snapshot.InAdjRoot, "incoming adjacency does not match the edge tree")
	}
	c.nodes[snapshot.NodeRoot], c.edges[id] = nodes, edges
	if schema, ok := c.schema(snapshot.SchemaRoot); ok {
		if err := ValidateSchemaSnapshot(schema, nodes, edges); err != nil {
			c.issue("schema-violation", "", branch, id, err.Error())
		}
	}
	return true
}

func (c *fsckChecker) schema(id ObjectID) (SchemaSnapshot, bool) {
	data, ok := c.objects[id]
	if !ok {
		return SchemaSnapshot{}, false
	}
	var schema SchemaSnapshot
	if err := cbor.Unmarshal(data, &schema); err != nil {
		return SchemaSnapshot{}, false
	}
	return schema, true
}

func (c *fsckChecker) checkProllyTree(id ObjectID) ([]prollyTreeEntry, bool) {
	return c.checkProllyTreeObject(id, make(map[ObjectID]bool))
}

func (c *fsckChecker) checkProllyTreeObject(id ObjectID, visiting map[ObjectID]bool) ([]prollyTreeEntry, bool) {
	if visiting[id] {
		c.issue("prolly-cycle", "", "", id, "Prolly tree contains a cycle")
		return nil, false
	}
	visiting[id] = true
	defer delete(visiting, id)
	data, objectType, ok := c.objectAny(id, []string{prollyTreeLeafType, prollyTreeInternalType})
	if !ok {
		return nil, false
	}
	if objectType == prollyTreeLeafType {
		var leaf prollyTreeLeaf
		if err := cbor.Unmarshal(data, &leaf); err != nil {
			c.issue("invalid-prolly-leaf", "", "", id, "leaf cannot be decoded")
			return nil, false
		}
		encoded, err := canonicalObjectEncoding(leaf)
		if err != nil || !bytes.Equal(data, encoded) || !validProllyEntries(leaf.Entries, true) {
			c.issue("invalid-prolly-leaf", "", "", id, "leaf has invalid canonical ordering or fanout")
			return nil, false
		}
		return append([]prollyTreeEntry(nil), leaf.Entries...), true
	}
	var internal prollyTreeInternal
	if err := cbor.Unmarshal(data, &internal); err != nil {
		c.issue("invalid-prolly-internal", "", "", id, "internal node cannot be decoded")
		return nil, false
	}
	encoded, err := canonicalObjectEncoding(internal)
	if err != nil || !bytes.Equal(data, encoded) || !validProllyChildren(internal.Children) {
		c.issue("invalid-prolly-internal", "", "", id, "internal node has invalid canonical ordering or fanout")
		return nil, false
	}
	entries := make([]prollyTreeEntry, 0)
	for _, child := range internal.Children {
		childEntries, childOK := c.checkProllyTreeObject(child.Object, visiting)
		if !childOK || len(childEntries) == 0 || childEntries[len(childEntries)-1].Key != child.LastKey {
			c.issue("invalid-prolly-boundary", "", "", id, "child boundary does not match its last key")
			return nil, false
		}
		entries = append(entries, childEntries...)
	}
	if !validProllyEntries(entries, false) {
		c.issue("invalid-prolly-ordering", "", "", id, "tree entries are not globally ordered")
		return nil, false
	}
	return entries, true
}

func (c *fsckChecker) checkStagedState() {
	if c.stateDir != "" {
		c.staged = c.readStaged()
	}
	validator := &Repository{
		branches: c.branches, commits: c.commits, snapshots: c.snapshots,
		projections: c.nodes, edgeProjections: c.edges, objects: c.objects,
	}
	for _, branch := range sortedFsckStaged(c.staged) {
		staged := c.staged[branch]
		if !validRefName(branch) || staged.Branch != branch || !validLooseObjectID(staged.BaseCommit) ||
			(len(staged.Operations) == 0 && staged.TargetSchema == nil) {
			c.issue("invalid-staged-state", filepath.ToSlash(filepath.Join("staged", branch+".json")), branch, "", "staged state is malformed")
			continue
		}
		if _, exists := c.branches[branch]; !exists {
			c.issue("unknown-staged-branch", filepath.ToSlash(filepath.Join("staged", branch+".json")), branch, "", "staged state names no branch")
		}
		if _, exists := c.commits[staged.BaseCommit]; !exists {
			c.issue("missing-staged-base", filepath.ToSlash(filepath.Join("staged", branch+".json")), branch, staged.BaseCommit, "staged base commit is not reachable from a branch")
			continue
		}
		if head, exists := c.branches[branch]; exists && !c.commitReachable(head, staged.BaseCommit) {
			c.issue("invalid-staged-base", filepath.ToSlash(filepath.Join("staged", branch+".json")), branch, staged.BaseCommit, "staged base commit is not reachable from its branch")
			continue
		}
		normalized, err := normalizeStoredMutationOperations(staged.Operations)
		if err != nil || !reflect.DeepEqual(staged.Operations, normalized) {
			c.issue("invalid-staged-operations", filepath.ToSlash(filepath.Join("staged", branch+".json")), branch, "", "staged operations are not canonical")
			continue
		}
		if staged.TargetSchema != nil {
			normalizedSchema, err := staged.TargetSchema.Normalize()
			if err != nil || !reflect.DeepEqual(*staged.TargetSchema, normalizedSchema) {
				c.issue("invalid-staged-schema", filepath.ToSlash(filepath.Join("staged", branch+".json")), branch, "", "staged schema is not normalized")
				continue
			}
		}
		if _, _, err := validator.candidateGraphLocked(staged.BaseCommit, staged); err != nil {
			c.issue("invalid-staged-graph", filepath.ToSlash(filepath.Join("staged", branch+".json")), branch, "", err.Error())
		}
	}
}

func (c *fsckChecker) commitReachable(head, target ObjectID) bool {
	seen := make(map[ObjectID]struct{})
	queue := []ObjectID{head}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current == target {
			return true
		}
		if _, visited := seen[current]; visited {
			continue
		}
		seen[current] = struct{}{}
		for _, parent := range c.commits[current].Parents {
			if _, visited := seen[parent]; !visited {
				queue = append(queue, parent)
			}
		}
	}
	return false
}

func (c *fsckChecker) readStaged() map[string]StagedMutationSet {
	staged := make(map[string]StagedMutationSet)
	root := filepath.Join(c.stateDir, "staged")
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			c.issue("read-staged-state", "staged", "", "", "cannot walk staged state")
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			c.issue("invalid-staged-file", "staged", "", "", "cannot determine staged branch")
			return nil
		}
		name := strings.TrimSuffix(filepath.ToSlash(relative), ".json")
		displayPath := filepath.ToSlash(filepath.Join("staged", relative))
		if !entry.Type().IsRegular() || filepath.Ext(path) != ".json" || !validRefName(name) {
			c.issue("invalid-staged-file", displayPath, name, "", "staged state must be a regular JSON file")
			return nil
		}
		data, ok := c.controlFile(path, displayPath)
		if !ok {
			return nil
		}
		var mutationSet StagedMutationSet
		if err := json.Unmarshal(data, &mutationSet); err != nil || mutationSet.Branch != name {
			c.issue("invalid-staged-file", displayPath, name, "", "staged state cannot be decoded")
			return nil
		}
		staged[name] = mutationSet
		return nil
	})
	if os.IsNotExist(err) {
		c.issue("missing-staged-directory", "staged", "", "", "staged state directory is missing")
	} else if err != nil {
		c.issue("read-staged-state", "staged", "", "", "cannot read staged state")
	}
	return staged
}

func (c *fsckChecker) checkMergeTransactions() {
	if c.stateDir == "" {
		return
	}
	root := filepath.Join(c.stateDir, "merge")
	targets := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			c.issue("read-merge-state", "merge", "", "", "cannot walk merge state")
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		relative, _ := filepath.Rel(root, path)
		displayPath := filepath.ToSlash(filepath.Join("merge", relative))
		if !entry.Type().IsRegular() || filepath.Ext(path) != ".json" {
			c.issue("invalid-merge-file", displayPath, "", "", "merge state must be a regular JSON file")
			return nil
		}
		data, ok := c.controlFile(path, displayPath)
		if !ok {
			return nil
		}
		var state persistedMergeTransaction
		if err := json.Unmarshal(data, &state); err != nil {
			c.issue("invalid-merge-state", displayPath, "", "", "merge state cannot be decoded")
			return nil
		}
		if !validRefName(state.TargetBranch) || filepath.ToSlash(relative) != mergeStateFilename(state.TargetBranch) {
			c.issue("invalid-merge-binding", displayPath, state.TargetBranch, "", "merge transaction target does not match its state file")
		}
		if previous, duplicate := targets[state.TargetBranch]; duplicate {
			c.issue("duplicate-merge-binding", displayPath, state.TargetBranch, "", "merge transaction target is also bound by "+previous)
		} else {
			targets[state.TargetBranch] = displayPath
		}
		repo := &Repository{
			branches: c.branches, commits: c.commits, snapshots: c.snapshots, projections: c.nodes,
			edgeProjections: c.edges, objects: c.objects,
		}
		stagedSnapshotValid := true
		if state.Transaction.Resolved {
			stagedSnapshotValid = state.Transaction.StagedSnapshot != "" &&
				c.checkSnapshot(state.Transaction.StagedSnapshot, state.TargetBranch)
		}
		if !stagedSnapshotValid || !repo.validPersistedMergeTransactionLocked(state) {
			c.issue("invalid-merge-binding", displayPath, state.TargetBranch, "", "merge transaction binding is invalid or stale")
		}
		return nil
	})
	if os.IsNotExist(err) {
		c.issue("missing-merge-directory", "merge", "", "", "merge state directory is missing")
	} else if err != nil {
		c.issue("read-merge-state", "merge", "", "", "cannot read merge state")
	}
}

func sortedFsckBranches(branches map[string]ObjectID) []string {
	names := make([]string, 0, len(branches))
	for name := range branches {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedFsckStaged(staged map[string]StagedMutationSet) []string {
	names := make([]string, 0, len(staged))
	for name := range staged {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
