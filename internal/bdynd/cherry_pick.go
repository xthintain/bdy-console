package bdynd

import "fmt"

type CherryPickResult struct {
	Commit    string
	Conflicts []string
}

func CherryPick(r Repo, ref string) (CherryPickResult, error) {
	oid, err := ResolveRef(r, ref)
	if err != nil {
		return CherryPickResult{}, err
	}
	commit, err := ReadCommit(r, oid)
	if err != nil {
		return CherryPickResult{}, err
	}
	conflicts, err := replayCommit(r, commit)
	if len(conflicts) > 0 {
		return CherryPickResult{Conflicts: conflicts}, fmt.Errorf("cherry-pick conflicts: %v", conflicts)
	}
	if err != nil {
		return CherryPickResult{}, err
	}
	next, err := Commit(r, CommitOptions{Message: commit.Message})
	if err != nil {
		return CherryPickResult{}, err
	}
	return CherryPickResult{Commit: next.OID}, nil
}
