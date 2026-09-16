package httpapi

import (
	"net/http"
)

// handleJoin — POST /v1/join. Пригласительная ссылка в закрытый ленточный канал.
// Пользователь вступает по ссылке; если канал в режиме одобрения,
// worker автоматически одобряет chat_join_request.
// Требует authMW; возвращает 503 если feedChannelID=0 или bot=nil.
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
