package httpapi

import (
	"net/http"
)

// GET /v1/join — пригласительная ссылка в закрытый ленточный канал.
// Пользователь вступает по ссылке; если канал в режиме одобрения,
// worker автоматически одобряет chat_join_request.
func (s *Server) handleJoin(w http.ResponseWriter, r *http.Request) {
	if s.feedChannelID == 0 || s.bot == nil {
		writeErr(w, http.StatusServiceUnavailable, "feed channel not configured")
		return
	}
	link, err := s.bot.ExportChatInviteLink(s.feedChannelID)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "bot api: exportChatInviteLink failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"invite_link": link})
}