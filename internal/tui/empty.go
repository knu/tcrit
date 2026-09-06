package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func (m AppModel) renderEmptyReview() (string, []modalMouseRegion) {
	body := "TCrit: [" + m.reviewScopeLabel() + "]\n\nNo changes remain in this review.\n\nq: finish review"
	body = lipgloss.NewStyle().Width(m.width).Height(m.height).Render(body)
	if m.modal != noModal {
		return m.renderWithModalLayout(body)
	}
	return body, nil
}

func (m *AppModel) handleEmptyReviewClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if m.modal != finishModal || msg.Mouse().Button != tea.MouseLeft {
		return m, nil
	}
	_, regions := m.renderEmptyReview()
	for _, region := range regions {
		mouse := msg.Mouse()
		if mouse.X >= region.rect.left && mouse.X < region.rect.right && mouse.Y >= region.rect.top && mouse.Y < region.rect.bottom {
			m.modalFocus = region.action.focus
			return m.handleFinishModal(tea.KeyPressMsg{Code: tea.KeyEnter})
		}
	}
	return m, nil
}
