package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		jsonErr(w, 400, "username and password are required")
		return
	}

	if len(req.Username) < 3 {
		jsonErr(w, 400, "username must be at least 3 characters")
		return
	}

	if len(req.Password) < 6 {
		jsonErr(w, 400, "password must be at least 6 characters")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		jsonErr(w, 500, "failed to hash password")
		return
	}

	user, err := h.db.CreateUser(req.Username, string(hash))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			jsonErr(w, 409, "username already exists")
			return
		}
		jsonErr(w, 500, err.Error())
		return
	}

	token := h.sessions.Create(user.ID)
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   7 * 24 * 60 * 60,
	})

	jsonResp(w, 201, user)
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}

	user, err := h.db.GetUserByUsername(req.Username)
	if err != nil {
		jsonErr(w, 401, "invalid username or password")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		jsonErr(w, 401, "invalid username or password")
		return
	}

	token := h.sessions.Create(user.ID)
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   7 * 24 * 60 * 60,
	})

	jsonResp(w, 200, user)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session")
	if err == nil && cookie.Value != "" {
		h.sessions.Delete(cookie.Value)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})

	jsonResp(w, 200, map[string]string{"message": "logged out"})
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	user, err := h.db.GetUserByID(userID)
	if err != nil {
		jsonErr(w, 404, "user not found")
		return
	}
	jsonResp(w, 200, user)
}

func (h *Handler) changePassword(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}

	if len(req.NewPassword) < 6 {
		jsonErr(w, 400, "new password must be at least 6 characters")
		return
	}

	// Get current password hash
	passwordHash, err := h.db.GetPasswordHash(userID)
	if err != nil {
		jsonErr(w, 404, "user not found")
		return
	}

	// Verify old password
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.OldPassword)); err != nil {
		jsonErr(w, 401, "incorrect old password")
		return
	}

	// Hash new password
	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		jsonErr(w, 500, "failed to hash password")
		return
	}

	// Update password
	if err := h.db.UpdatePassword(userID, string(newHash)); err != nil {
		jsonErr(w, 500, "failed to update password")
		return
	}

	jsonResp(w, 200, map[string]string{"message": "password updated"})
}

func (h *Handler) deleteAccount(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)

	// Delete user's data
	h.db.DeleteUser_data(userID)

	// Clear session
	cookie, err := r.Cookie("session")
	if err == nil && cookie.Value != "" {
		h.sessions.Delete(cookie.Value)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})

	jsonResp(w, 200, map[string]string{"message": "account deleted"})
}
