package service

import (
	"sort"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

// A title's mapping is computed, never copied: from its own evidence and
// that of the titles directly linked to it that are linked to nothing else
// (see groupAnchor for which wins). A title fixed by hand keeps its fix; a
// title whose linked titles disagree falls back to its own evidence.

// evidenceRank orders evidence kinds, strongest first.
func evidenceRank(kind string) int {
	switch kind {
	case repository.EvidenceManual:
		return 3
	case repository.EvidenceNetflixID:
		return 2
	case repository.EvidenceMatch:
		return 1
	}
	return 0
}

// bestEvidence is a subject's strongest evidence (nil if none).
func bestEvidence(list []repository.Evidence) *repository.Evidence {
	var best *repository.Evidence
	for i := range list {
		if best == nil || evidenceRank(list[i].Kind) > evidenceRank(best.Kind) {
			best = &list[i]
		}
	}
	return best
}

func evidenceMember(e repository.Evidence) mappingMember {
	m := mappingMember{netflix: e.Type == repository.SubjectNetflix, id: e.ID, ref: e.Ref, manual: e.Kind == repository.EvidenceManual}
	m.matched = !m.manual && m.ref.ID != ""
	m.exact = m.matched && e.Kind == repository.EvidenceNetflixID
	return m
}

// mappingOutcome is a computed mapping and whether the deciding titles
// disagreed.
type mappingOutcome struct {
	repository.Mapping
	shared   bool // taken from a linked title
	conflict bool
}

// computeMapping derives a subject's mapping from in.
func computeMapping(in repository.MappingInputs, s repository.Subject) mappingOutcome {
	cur := in.Current[s]
	own := bestEvidence(in.Evidence[s])
	var set []mappingMember
	if own != nil {
		set = append(set, evidenceMember(*own))
	}
	for _, nb := range in.Neighbours(s) {
		// A linked title also linked elsewhere (one Netflix title for a show
		// and its revival) says nothing certain about this one.
		if len(in.Neighbours(nb)) != 1 {
			continue
		}
		if e := bestEvidence(in.Evidence[nb]); e != nil {
			set = append(set, evidenceMember(*e))
		}
	}
	anchor, ok, conflict := groupAnchor(set)
	if own != nil && own.Kind == repository.EvidenceManual {
		// A fix by hand stands, even against a linked title's fix.
		return mappingOutcome{Mapping: repository.Mapping{Ref: own.Ref, Status: repository.MatchStatusManual, Source: repository.MatchStatusManual}, conflict: conflict}
	}
	self := ok && anchor.netflix == (s.Type == repository.SubjectNetflix) && anchor.id == s.ID
	switch {
	case ok && !self:
		m := repository.Mapping{Ref: anchor.ref, Status: repository.MatchStatusMatched}
		if s.Type == repository.SubjectNetflix {
			m.Source, m.ViaTitleID = MatchSourceFlixPatrolTitle, anchor.id
		} else {
			m.Source = MatchSourceNetflixTitle
		}
		return mappingOutcome{Mapping: m, shared: true}
	case own != nil && own.Ref.ID != "":
		m := repository.Mapping{Ref: own.Ref, Status: repository.MatchStatusMatched, Source: own.Source}
		if s.Type == repository.SubjectNetflix {
			m.ViaTitleID = own.ViaTitleID
		} else {
			m.OwnMatch = own.Kind == repository.EvidenceMatch
		}
		return mappingOutcome{Mapping: m, conflict: conflict}
	}
	// Nothing says what it is: a title that had a mapping is due for matching.
	m := repository.Mapping{}
	if cur.Status == repository.MatchStatusMatched || cur.Status == repository.MatchStatusManual {
		m.Status = repository.MatchStatusPending
	}
	return mappingOutcome{Mapping: m, conflict: conflict}
}

// derivedAction names a change the writer made to a title because of
// another title's change.
func derivedAction(cur repository.Mapping, out mappingOutcome) string {
	wasShared := cur.Source == MatchSourceFlixPatrolTitle || cur.Source == MatchSourceNetflixTitle
	switch {
	case out.shared:
		return repository.MappingActionAdopt
	case out.Ref.ID == "" && cur.Ref.ID != "", wasShared:
		// What it had taken from a linked title went away (it falls back
		// to its own evidence, if any).
		return repository.MappingActionUnshare
	}
	return repository.MappingActionMatch
}

// recomputeStats counts what a recompute changed.
type recomputeStats struct {
	NetflixAdopted    int // Netflix titles that took a linked title's mapping
	FlixPatrolAdopted int
	Conflicts         int // titles whose linked titles disagree
}

// recompute computes and applies the mappings of seeds and the titles
// linked to them; seeds' changes are recorded under action ("" = derived,
// like the rest).
func recompute(tx *repository.MappingTx, seeds []repository.Subject, action string) (recomputeStats, error) {
	var st recomputeStats
	links, err := tx.Links()
	if err != nil {
		return st, err
	}
	probe := repository.MappingInputs{Links: links}
	isSeed := map[repository.Subject]bool{}
	affected := map[repository.Subject]bool{}
	for _, s := range seeds {
		isSeed[s] = true
		affected[s] = true
		for _, nb := range probe.Neighbours(s) {
			affected[nb] = true
		}
	}
	list := make([]repository.Subject, 0, len(affected))
	for s := range affected {
		list = append(list, s)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Type != list[j].Type {
			return list[i].Type < list[j].Type
		}
		return list[i].ID < list[j].ID
	})
	in, err := tx.Inputs(list)
	if err != nil {
		return st, err
	}
	for _, s := range list {
		cur, ok := in.Current[s]
		if !ok {
			continue // gone
		}
		out := computeMapping(in, s)
		if out.conflict {
			st.Conflicts++
		}
		act := action
		if !isSeed[s] || act == "" {
			act = derivedAction(cur, out)
		}
		before := len(tx.Changes())
		if err := tx.Apply(s, cur, out.Mapping, act); err != nil {
			return st, err
		}
		if len(tx.Changes()) > before && act == repository.MappingActionAdopt {
			if s.Type == repository.SubjectNetflix {
				st.NetflixAdopted++
			} else {
				st.FlixPatrolAdopted++
			}
		}
	}
	return st, nil
}
