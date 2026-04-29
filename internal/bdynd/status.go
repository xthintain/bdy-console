package bdynd

type StatusResult struct {
	Added    []string
	Modified []string
	Deleted  []string
}

func (s StatusResult) Clean() bool {
	return len(s.Added)+len(s.Modified)+len(s.Deleted) == 0
}

func Status(r Repo) (StatusResult, error) {
	idx, err := LoadIndex(r)
	if err != nil {
		return StatusResult{}, err
	}
	head, err := HeadCommit(r)
	if err != nil || head == "" {
		var st StatusResult
		for _, entry := range sortedIndexEntries(idx) {
			st.Added = append(st.Added, entry.Path)
		}
		return st, nil
	}
	c, err := ReadCommit(r, head)
	if err != nil {
		return StatusResult{}, err
	}
	headEntries := map[string]IndexEntry{}
	for _, entry := range c.Entries {
		headEntries[entry.Path] = entry
	}
	var st StatusResult
	for _, entry := range sortedIndexEntries(idx) {
		headEntry, ok := headEntries[entry.Path]
		if !ok {
			st.Added = append(st.Added, entry.Path)
			continue
		}
		if entry.OID != headEntry.OID || entry.LFSOID != headEntry.LFSOID || entry.Kind != headEntry.Kind {
			st.Modified = append(st.Modified, entry.Path)
		}
		delete(headEntries, entry.Path)
	}
	for path := range headEntries {
		st.Deleted = append(st.Deleted, path)
	}
	return st, nil
}
