package tui

import "github.com/knu/tcrit/internal/review"

func (m *AppModel) restoreRound() error {
	if m.session == nil {
		return nil
	}
	files := make(map[string]review.SnapshotFile, len(m.tabs))
	for _, tab := range m.tabs {
		if tab.isBinary || tab.outsideChanges {
			continue
		}
		if tab.isDeleted {
			files[tab.path] = review.SnapshotFile{}
			continue
		}
		doc, err := m.loadDocument(tab.path)
		if err != nil {
			return err
		}
		files[tab.path] = review.SnapshotFile{Content: doc.Content, Partial: doc.Known != nil}
	}
	rawDiff := ""
	if m.patch != nil {
		rawDiff = m.patch.Raw
	}
	if err := m.session.BeginRound(files, rawDiff); err != nil {
		return err
	}
	m.newFeedback = m.session.CJ.RoundState.NewFeedback
	return nil
}
