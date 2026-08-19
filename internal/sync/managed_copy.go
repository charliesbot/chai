package sync

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/charliesbot/chai/internal/hash"
)

type managedDesired struct {
	Name       string
	ChangeName string
	Source     string
	Content    []byte
}

type reconciliationPolicy struct {
	RemoveStale      bool
	ProtectUnmanaged bool
	ProtectDirty     bool
	PreserveDeclined bool
	Force            bool
	Prompt           PromptFunc
}

type reconciliationResult struct {
	changes   itemChanges
	preserved []string
	skipped   []string
}

type unmanagedDestinationError struct {
	Path string
}

func (err *unmanagedDestinationError) Error() string {
	return fmt.Sprintf("destination %s is not managed by chai", err.Path)
}

type declinedDestinationError struct {
	Path  string
	Stale bool
}

func (err *declinedDestinationError) Error() string {
	return fmt.Sprintf("modified destination %s was preserved", err.Path)
}

type managedDestinationAdapter interface {
	matches(fs.DirEntry) bool
	digest(string) (string, error)
	install(managedDesired, string, bool) (string, error)
	remove(string) error
}

type fileDestinationAdapter struct {
	extension string
}

func (adapter fileDestinationAdapter) matches(entry fs.DirEntry) bool {
	return !entry.IsDir() && filepath.Ext(entry.Name()) == adapter.extension
}

func (fileDestinationAdapter) digest(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return hash.Sum(data), nil
}

func (fileDestinationAdapter) install(desired managedDesired, destination string, _ bool) (string, error) {
	data := desired.Content
	if desired.Source != "" {
		var err error
		data, err = os.ReadFile(desired.Source)
		if err != nil {
			return "", fmt.Errorf("reading %s: %w", desired.Source, err)
		}
	}
	if err := atomicWrite(destination, data); err != nil {
		return "", err
	}
	return hash.Sum(data), nil
}

func (fileDestinationAdapter) remove(path string) error {
	return os.Remove(path)
}

type directoryDestinationAdapter struct{}

func (directoryDestinationAdapter) matches(entry fs.DirEntry) bool {
	return entry.IsDir()
}

func (directoryDestinationAdapter) digest(path string) (string, error) {
	return dirHash(path)
}

func (directoryDestinationAdapter) install(desired managedDesired, destination string, existed bool) (string, error) {
	staging, err := os.MkdirTemp(filepath.Dir(destination), "."+desired.Name+".tmp-")
	if err != nil {
		return "", fmt.Errorf("creating staging directory for %s: %w", destination, err)
	}
	sum, err := copyAndHashDir(desired.Source, staging)
	if err != nil {
		_ = os.RemoveAll(staging)
		return "", err
	}
	if err := replaceDirectory(staging, destination, existed); err != nil {
		return "", err
	}
	return sum, nil
}

func (directoryDestinationAdapter) remove(path string) error {
	return os.RemoveAll(path)
}

func reconcileManagedFiles(
	destinationDir string,
	extension string,
	desired []managedDesired,
	hashDB hash.DB,
	policy reconciliationPolicy,
) (reconciliationResult, error) {
	return reconcileManagedDestinations(destinationDir, desired, fileDestinationAdapter{extension: extension}, hashDB, policy)
}

func reconcileManagedDirectories(
	destinationDir string,
	desired []managedDesired,
	hashDB hash.DB,
	policy reconciliationPolicy,
) (reconciliationResult, error) {
	return reconcileManagedDestinations(destinationDir, desired, directoryDestinationAdapter{}, hashDB, policy)
}

func instructionReconciliationPolicy(opts Options) reconciliationPolicy {
	return reconciliationPolicy{
		ProtectDirty:     true,
		PreserveDeclined: true,
		Force:            opts.Force,
		Prompt:           opts.Prompt,
	}
}

func skillReconciliationPolicy(opts Options) reconciliationPolicy {
	return reconciliationPolicy{
		RemoveStale:      true,
		ProtectUnmanaged: true,
		ProtectDirty:     true,
		Force:            opts.Force,
		Prompt:           opts.Prompt,
	}
}

func generatedFileReconciliationPolicy() reconciliationPolicy {
	return reconciliationPolicy{RemoveStale: true}
}

func reconcileManagedDestinations(
	destinationDir string,
	desired []managedDesired,
	adapter managedDestinationAdapter,
	hashDB hash.DB,
	policy reconciliationPolicy,
) (reconciliationResult, error) {
	result := reconciliationResult{changes: newItemChanges()}
	if err := os.MkdirAll(destinationDir, 0755); err != nil {
		return result, fmt.Errorf("creating directory %s: %w", destinationDir, err)
	}

	expected := make(map[string]bool, len(desired))
	for _, item := range desired {
		expected[filepath.Join(destinationDir, item.Name)] = true
	}
	if policy.RemoveStale {
		entries, err := os.ReadDir(destinationDir)
		if err != nil {
			return result, fmt.Errorf("reading %s: %w", destinationDir, err)
		}
		for _, entry := range entries {
			if !adapter.matches(entry) {
				continue
			}
			path := filepath.Join(destinationDir, entry.Name())
			if expected[path] {
				continue
			}
			stored, managed := hashDB[path]
			if !managed {
				result.preserved = append(result.preserved, entry.Name())
				continue
			}
			if policy.ProtectDirty && !policy.Force {
				dirty, err := managedDestinationDirty(adapter, path, stored)
				if err != nil {
					return result, err
				}
				if dirty {
					accepted, err := confirmManagedDestination(policy.Prompt, path)
					if err != nil {
						return result, err
					}
					if !accepted {
						return result, &declinedDestinationError{Path: path, Stale: true}
					}
				}
			}
			if err := adapter.remove(path); err != nil {
				return result, fmt.Errorf("removing %s: %w", path, err)
			}
			delete(hashDB, path)
			result.changes.record(entry.Name(), itemRemoved)
		}
	}

	for _, item := range desired {
		destination := filepath.Join(destinationDir, item.Name)
		previousHash, managed := hashDB[destination]
		_, statErr := os.Stat(destination)
		existed := statErr == nil
		if statErr != nil && !os.IsNotExist(statErr) {
			return result, fmt.Errorf("checking %s: %w", destination, statErr)
		}
		if existed && !managed && policy.ProtectUnmanaged {
			return result, &unmanagedDestinationError{Path: destination}
		}
		if existed && managed && policy.ProtectDirty && !policy.Force {
			dirty, err := managedDestinationDirty(adapter, destination, previousHash)
			if err != nil {
				return result, err
			}
			if dirty {
				accepted, err := confirmManagedDestination(policy.Prompt, destination)
				if err != nil {
					return result, err
				}
				if !accepted {
					if policy.PreserveDeclined {
						result.skipped = append(result.skipped, destination)
						continue
					}
					return result, &declinedDestinationError{Path: destination}
				}
			}
		}

		currentHash, err := adapter.install(item, destination, existed)
		if err != nil {
			return result, err
		}
		hashDB[destination] = currentHash
		changeName := item.ChangeName
		if changeName == "" {
			changeName = item.Name
		}
		switch {
		case !managed:
			result.changes.record(changeName, itemAdded)
		case !existed || previousHash != currentHash:
			result.changes.record(changeName, itemUpdated)
		default:
			result.changes.record(changeName, itemUnchanged)
		}
	}
	return result, nil
}

func managedDestinationDirty(adapter managedDestinationAdapter, path, stored string) (bool, error) {
	current, err := adapter.digest(path)
	if err != nil {
		return false, fmt.Errorf("hashing managed destination %s: %w", path, err)
	}
	return current != stored, nil
}

func confirmManagedDestination(prompt PromptFunc, path string) (bool, error) {
	if prompt == nil {
		return false, &DirtyError{Files: []string{path}}
	}
	return prompt(path)
}
