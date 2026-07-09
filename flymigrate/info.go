package flymigrate

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// Info retrieves consolidated status for all local and DB migrations.
func (rf *Migrator) Info(ctx context.Context) ([]StatusEntry, error) {
	db, selfCreated, err := rf.getDB(ctx)
	if err != nil {
		return nil, err
	}
	if selfCreated {
		defer db.Close()
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("rapidfly/migrate: failed to acquire connection: %w", err)
	}
	defer conn.Close()

	if err := rf.ensureMetadataTable(ctx, conn); err != nil {
		return nil, err
	}

	localMigs, err := rf.loadMigrations()
	if err != nil {
		return nil, err
	}

	history, err := rf.fetchHistory(ctx, conn)
	if err != nil {
		return nil, err
	}

	var statusList []StatusEntry

	historyByScript := make(map[string][]HistoryEntry)
	for _, entry := range history {
		historyByScript[entry.Script] = append(historyByScript[entry.Script], entry)
	}

	historyByVersion := make(map[string][]HistoryEntry)
	for _, entry := range history {
		if entry.Type == "SQL" && entry.Version != nil {
			historyByVersion[*entry.Version] = append(historyByVersion[*entry.Version], entry)
		}
	}

	localMap := make(map[string]*Migration)
	localVersionMap := make(map[string]*Migration)
	for _, lm := range localMigs {
		localMap[lm.Script] = lm
		if lm.Type == "SQL" {
			localVersionMap[lm.Version] = lm
		}
	}

	for _, lm := range localMigs {
		var state string
		var installedOn *time.Time

		if lm.Type == "SQL" {
			entries := historyByVersion[lm.Version]
			if len(entries) == 0 {
				state = "Pending"
			} else {
				latest := entries[len(entries)-1]
				if !latest.Success {
					state = "Failed"
				} else if !rf.config.SkipChecksum && latest.Checksum != lm.Checksum {
					state = "ChecksumMismatch"
				} else {
					state = "Success"
				}
				installedOn = &latest.InstalledOn
			}
		} else {
			entries := historyByScript[lm.Script]
			if len(entries) == 0 {
				state = "Pending"
			} else {
				latest := entries[len(entries)-1]
				if !latest.Success {
					state = "Failed"
				} else if !rf.config.SkipChecksum && latest.Checksum != lm.Checksum {
					state = "ChecksumMismatch"
				} else {
					state = "Success"
				}
				installedOn = &latest.InstalledOn
			}
		}

		statusList = append(statusList, StatusEntry{
			Version:     lm.Version,
			Description: lm.Description,
			Script:      lm.Script,
			Type:        lm.Type,
			State:       state,
			InstalledOn: installedOn,
		})
	}

	for _, entry := range history {
		if entry.Type == "SQL" && entry.Version != nil {
			if _, localExists := localVersionMap[*entry.Version]; !localExists {
				found := false
				for _, st := range statusList {
					if st.Type == "SQL" && st.Version == *entry.Version {
						found = true
						break
					}
				}
				if !found {
					t := entry.InstalledOn
					statusList = append(statusList, StatusEntry{
						Version:     *entry.Version,
						Description: entry.Description,
						Script:      entry.Script,
						Type:        entry.Type,
						State:       "Missing",
						InstalledOn: &t,
					})
				}
			}
		} else if entry.Type == "REPEATABLE" {
			if _, localExists := localMap[entry.Script]; !localExists {
				found := false
				for _, st := range statusList {
					if st.Type == "REPEATABLE" && st.Script == entry.Script {
						found = true
						break
					}
				}
				if !found {
					t := entry.InstalledOn
					statusList = append(statusList, StatusEntry{
						Version:     "",
						Description: entry.Description,
						Script:      entry.Script,
						Type:        entry.Type,
						State:       "Missing",
						InstalledOn: &t,
					})
				}
			}
		}
	}

	sort.Slice(statusList, func(i, j int) bool {
		if statusList[i].Type == "SQL" && statusList[j].Type == "SQL" {
			return compareVersions(statusList[i].Version, statusList[j].Version) < 0
		}
		if statusList[i].Type == "SQL" && statusList[j].Type != "SQL" {
			return true
		}
		if statusList[i].Type != "SQL" && statusList[j].Type == "SQL" {
			return false
		}
		return statusList[i].Script < statusList[j].Script
	})

	return statusList, nil
}
