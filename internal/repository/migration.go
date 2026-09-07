package repository

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/autonomous-bits/spool/graphcontract"
	"github.com/fxamacker/cbor/v2"
	"github.com/gofrs/flock"
	"github.com/pelletier/go-toml/v2"
)

// MigrationResult describes the outcome of an explicit repository format migration.
type MigrationResult struct {
	FromVersion int    `json:"fromVersion"`
	ToVersion   int    `json:"toVersion"`
	BackupPath  string `json:"backupPath"`
}

// MigrateRepositoryFormat upgrades a Spool repository from format_version 'from'
// to format_version 'to'. It validates the migration path, acquires the repository
// lock, creates a durable backup, canonicalizes the commit DAG, remaps refs and
// reflogs, updates config.toml, invalidates cached projections, and verifies integrity.
func MigrateRepositoryFormat(stateDir string, from, to int) (*MigrationResult, error) {
	if from != 1 || to != repositoryFormatVersion {
		return nil, fmt.Errorf("unsupported migration path: from version %d to version %d", from, to)
	}

	lock := flock.New(filepath.Join(stateDir, "repository.lock"))
	locked, err := lock.TryLock()
	if err != nil {
		return nil, fmt.Errorf("lock repository for migration: %w", err)
	}
	if !locked {
		return nil, ErrMergeRepositoryLocked
	}
	defer func() { _ = lock.Unlock() }()

	configPath := filepath.Join(stateDir, "config.toml")
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read config.toml: %w", err)
	}

	var conf map[string]any
	if err := toml.Unmarshal(configData, &conf); err != nil {
		return nil, fmt.Errorf("decode config.toml: %w", err)
	}

	var version int64
	switch v := conf["format_version"].(type) {
	case int64:
		version = v
	case float64:
		version = int64(v)
	case int:
		version = int64(v)
	default:
		return nil, fmt.Errorf("unrecognized format_version: %v", conf["format_version"])
	}

	if version == int64(to) {
		return nil, fmt.Errorf("repository is already at format version %d", to)
	}
	if version != int64(from) {
		return nil, fmt.Errorf("repository format version is %d, want %d", version, from)
	}

	// 1. Create durable backup
	backupDir := fmt.Sprintf("%s.v1.backup-%s", stateDir, time.Now().UTC().Format("20060102-150405"))
	if err := copyDirectoryTree(stateDir, backupDir); err != nil {
		return nil, fmt.Errorf("create backup %s: %w", backupDir, err)
	}

	if err := migrateV1ToV2Locked(stateDir, conf, configPath); err != nil {
		return nil, err
	}

	return &MigrationResult{
		FromVersion: from,
		ToVersion:   to,
		BackupPath:  backupDir,
	}, nil
}

// MigrateRepositoryFormatV1ToV2 upgrades a format_version 1 Spool repository to
// format_version 2.
func MigrateRepositoryFormatV1ToV2(stateDir string) error {
	_, err := MigrateRepositoryFormat(stateDir, 1, 2)
	return err
}

func migrateV1ToV2Locked(stateDir string, conf map[string]any, configPath string) error {

	// 2. Load all stored objects
	type objectRecord struct {
		objectType string
		data       []byte
	}
	allObjects := make(map[ObjectID]objectRecord)
	cached := make(map[ObjectID][]byte)
	store := newLooseObjectStore(stateDir, &cached)

	// 2a. Read loose objects
	looseDir := filepath.Join(stateDir, "objects", "loose")
	_ = filepath.WalkDir(looseDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(looseDir, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		parts := strings.Split(rel, "/")
		if len(parts) != 2 || len(parts[0]) != 2 || len(parts[1]) != objectIDHexLength-2 {
			return nil
		}
		id := ObjectID(parts[0] + parts[1])
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		var env looseObjectEnvelope
		if err := cbor.Unmarshal(data, &env); err == nil {
			allObjects[id] = objectRecord{objectType: env.Type, data: env.Data}
		}
		return nil
	})

	// 2b. Read packed objects
	manifest, err := store.readPackManifest()
	if err == nil {
		for _, metadata := range manifest.Packs {
			packPath, err := store.packPath(metadata.ID)
			if err != nil {
				continue
			}
			index, err := store.openPackIndex(metadata)
			if err != nil {
				continue
			}
			_ = index.ForEach(func(entry PackIndexEntry) error {
				objType, data, readErr := readPackedObjectFile(metadata.ID, packPath, entry, metadata.ObjectCount)
				if readErr == nil {
					if _, exists := allObjects[entry.Object]; !exists {
						allObjects[entry.Object] = objectRecord{
							objectType: objType,
							data:       append([]byte(nil), data...),
						}
					}
				}
				return nil
			})
			_ = index.Close()
		}
	}

	// 3. Find and decode all commits
	rawCommits := make(map[ObjectID]graphcontract.Commit)
	for id, obj := range allObjects {
		if obj.objectType != "commit" {
			continue
		}
		var rawMap map[int]any
		if err := cbor.Unmarshal(obj.data, &rawMap); err != nil {
			continue
		}
		snapshotID, _ := rawMap[1].(string)
		message, _ := rawMap[3].(string)
		author, _ := rawMap[4].(string)

		var parents []ObjectID
		if rawMap[2] != nil {
			if slice, ok := rawMap[2].([]any); ok {
				for _, item := range slice {
					if pStr, okStr := item.(string); okStr {
						parents = append(parents, ObjectID(pStr))
					}
				}
			}
		}

		var commitTime time.Time
		switch t := rawMap[5].(type) {
		case time.Time:
			commitTime = t
		case uint64:
			commitTime = time.Unix(int64(t), 0).UTC()
		case int64:
			commitTime = time.Unix(t, 0).UTC()
		case int:
			commitTime = time.Unix(int64(t), 0).UTC()
		}

		rawCommits[id] = graphcontract.Commit{
			Snapshot: ObjectID(snapshotID),
			Parents:  parents,
			Message:  message,
			Author:   author,
			Time:     commitTime.UTC().Truncate(time.Second),
		}
	}

	// 4. Canonicalize commit records in topological order
	oldToNew := make(map[ObjectID]ObjectID)
	inProgress := make(map[ObjectID]bool)
	var migrateCommit func(id ObjectID) (ObjectID, error)
	migrateCommit = func(id ObjectID) (ObjectID, error) {
		if newID, ok := oldToNew[id]; ok {
			return newID, nil
		}
		if inProgress[id] {
			return "", fmt.Errorf("cyclic commit history detected at commit %s", id)
		}
		inProgress[id] = true
		defer func() { inProgress[id] = false }()

		c, ok := rawCommits[id]
		if !ok {
			return "", fmt.Errorf("commit %s not found in repository history", id)
		}
		newParents := make([]ObjectID, len(c.Parents))
		for i, p := range c.Parents {
			newP, err := migrateCommit(p)
			if err != nil {
				return "", err
			}
			newParents[i] = newP
		}

		migrated := graphcontract.Commit{
			Snapshot: c.Snapshot,
			Parents:  newParents,
			Message:  c.Message,
			Author:   c.Author,
			Time:     c.Time.UTC().Truncate(time.Second),
		}
		data, err := graphcontract.MarshalCommit(migrated)
		if err != nil {
			return "", fmt.Errorf("marshal canonical commit %s: %w", id, err)
		}
		newID := objectIDForEncoded("commit", data)
		oldToNew[id] = newID

		// Durably write new canonical commit to loose storage
		if _, err := store.putEncoded("commit", data); err != nil {
			return "", fmt.Errorf("persist loose commit %s: %w", newID, err)
		}
		return newID, nil
	}

	for id := range rawCommits {
		if _, err := migrateCommit(id); err != nil {
			return err
		}
	}

	// 5. Update branch references
	refsDir := filepath.Join(stateDir, "refs", "heads")
	refEntries, err := os.ReadDir(refsDir)
	if err != nil {
		return fmt.Errorf("read branch references: %w", err)
	}
	for _, entry := range refEntries {
		if entry.IsDir() {
			continue
		}
		refPath := filepath.Join(refsDir, entry.Name())
		refData, err := os.ReadFile(refPath)
		if err != nil {
			return fmt.Errorf("read branch ref %s: %w", entry.Name(), err)
		}
		oldRef := ObjectID(strings.TrimSpace(string(refData)))
		newRef, ok := oldToNew[oldRef]
		if !ok {
			return fmt.Errorf("branch %s points to unknown commit %s", entry.Name(), oldRef)
		}
		if err := writeDurableStateFile(refPath, []byte(string(newRef)+"\n")); err != nil {
			return fmt.Errorf("update branch ref %s: %w", entry.Name(), err)
		}
	}

	// 6. Update branch reflogs (skipping logs/HEAD which stores branch ref names, not commit IDs)
	headsLogsDir := filepath.Join(stateDir, "logs", "refs", "heads")
	if _, statErr := os.Stat(headsLogsDir); statErr == nil {
		err = filepath.WalkDir(headsLogsDir, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil || d.IsDir() {
				return nil
			}
			lines, err := readReflogLines(path)
			if err != nil {
				return err
			}
			var newLines []string
			for _, fields := range lines {
				if len(fields) == 3 && fields[2] == "switch" {
					newLines = append(newLines, strings.Join(fields[:], " "))
					continue
				}
				fromID := ObjectID(fields[0])
				toID := ObjectID(fields[1])
				if fromID != "" {
					if migratedFrom, ok := oldToNew[fromID]; ok {
						fromID = migratedFrom
					}
				}
				if toID != "" {
					if migratedTo, ok := oldToNew[toID]; ok {
						toID = migratedTo
					}
				}
				fields[0] = string(fromID)
				fields[1] = string(toID)
				newLines = append(newLines, strings.Join(fields[:], " "))
			}
			content := strings.Join(newLines, "\n") + "\n"
			return writeDurableStateFile(path, []byte(content))
		})
		if err != nil {
			return fmt.Errorf("update reflogs: %w", err)
		}
	}

	// 7. Update repository configuration to format_version 2 and remap remote branch tracking
	conf["format_version"] = repositoryFormatVersion
	if rb, ok := conf["remote_branches"].(map[string]any); ok {
		for _, val := range rb {
			if entry, ok := val.(map[string]any); ok {
				if oldHead, ok := entry["remote_head_commit"].(string); ok && oldHead != "" {
					if newHead, ok := oldToNew[ObjectID(oldHead)]; ok {
						entry["remote_head_commit"] = string(newHead)
					}
				}
			}
		}
	}
	newConfigData, err := toml.Marshal(conf)
	if err != nil {
		return fmt.Errorf("encode config.toml: %w", err)
	}
	if err := writeDurableStateFile(configPath, newConfigData); err != nil {
		return fmt.Errorf("write config.toml: %w", err)
	}

	// 8. Invalidate previous SQLite projection so it cleanly rebuilds for the active branch
	_ = os.Remove(filepath.Join(stateDir, "graph.db"))

	// 9. Run full fsck integrity check
	report, err := FsckRepository(stateDir)
	if err != nil {
		return fmt.Errorf("fsck verification error: %w", err)
	}
	if !report.Valid {
		var msgs []string
		for _, diag := range report.Diagnostics {
			msgs = append(msgs, fmt.Sprintf("[%s] %s: %s", diag.Code, diag.Path, diag.Detail))
		}
		return fmt.Errorf("repository integrity validation failed:\n%s", strings.Join(msgs, "\n"))
	}

	return nil
}

func copyDirectoryTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o600)
	})
}
