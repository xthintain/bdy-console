package repo

import "sort"

type Status struct {
	Added     []string
	Modified  []string
	Deleted   []string
	Unchanged []string
}

func Diff(base, current Manifest) Status {
	baseMap := base.Map()
	curMap := current.Map()
	var st Status
	for path, cur := range curMap {
		old, ok := baseMap[path]
		switch {
		case !ok:
			st.Added = append(st.Added, path)
		case old.MD5 != cur.MD5 || old.Size != cur.Size || old.IsDir != cur.IsDir:
			st.Modified = append(st.Modified, path)
		default:
			st.Unchanged = append(st.Unchanged, path)
		}
	}
	for path := range baseMap {
		if _, ok := curMap[path]; !ok {
			st.Deleted = append(st.Deleted, path)
		}
	}
	sort.Strings(st.Added)
	sort.Strings(st.Modified)
	sort.Strings(st.Deleted)
	sort.Strings(st.Unchanged)
	return st
}

func Conflicts(base, local, remote Manifest) []string {
	localDiff := Diff(base, local)
	remoteDiff := Diff(base, remote)
	changedLocal := changeSet(localDiff)
	changedRemote := changeSet(remoteDiff)
	var out []string
	for p := range changedLocal {
		if changedRemote[p] {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

func changeSet(st Status) map[string]bool {
	out := map[string]bool{}
	for _, p := range st.Added {
		out[p] = true
	}
	for _, p := range st.Modified {
		out[p] = true
	}
	for _, p := range st.Deleted {
		out[p] = true
	}
	return out
}
