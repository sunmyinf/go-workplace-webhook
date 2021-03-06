package webhook

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/sunmyinf/go-workplace/decode"
)

// Server is to launch server to serve workplace webhook callback request.
type Server struct {
	Secret            string
	AccessToken       string
	VerificationToken string
	objectHandlers    map[string]map[decode.Object]func(decode.RequestBody) error
	mux               *http.ServeMux
}

// NewServer return workplace webhook server instance.
// Handler has been registered to it as '/webhook' pattern by default.
func NewServer(secret, accessToken, verificationToken string) *Server {
	ws := &Server{
		Secret:            secret,
		AccessToken:       accessToken,
		VerificationToken: verificationToken,
		mux:               http.NewServeMux(),
	}

	// Workplace webhook gets webhook to verify server
	ws.mux.HandleFunc("/webhook", ws.webhookHandlerFunc)
	return ws
}

// HandleObjectFunc registers handler by object to Server instance.
// If handler of specified object was registered, override it by new one.
func (ws *Server) HandleObjectFunc(pattern string, object decode.Object, objectHandler func(decode.RequestBody) error) {
	if ws.objectHandlers == nil {
		ws.objectHandlers = make(map[string]map[decode.Object]func(decode.RequestBody) error)
	}
	if ws.objectHandlers[pattern] == nil {
		ws.objectHandlers[pattern] = make(map[decode.Object]func(decode.RequestBody) error)
	}
	ws.objectHandlers[pattern][object] = objectHandler
}

// HandleFunc registers the handler function for the given pattern.
func (ws *Server) HandleFunc(pattern string, handler func(w http.ResponseWriter, r *http.Request)) {
	ws.mux.HandleFunc(pattern, handler)
}

// ListenAndServe listens on the TCP network address srv.Addr and then
// calls Serve to handle requests on incoming connections.
func (ws *Server) ListenAndServe(addr string) error {
	for pattern, _ := range ws.objectHandlers {
		ws.mux.HandleFunc(pattern, ws.getObjectHandlerFunc(pattern))
	}
	server := &http.Server{Addr: addr, Handler: ws.mux}
	return server.ListenAndServe()
}

func (ws *Server) webhookHandlerFunc(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Workplace webhook gets with some quereis to verify server
		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.FormValue("hub.mode") == "subscribe" && r.FormValue("hub.verify_token") == ws.VerificationToken {
			w.Write([]byte(r.FormValue("hub.challenge")))
		} else {
			w.WriteHeader(http.StatusForbidden)
		}
	default:
		w.WriteHeader(http.StatusForbidden)
	}
	return
}

func (ws *Server) getObjectHandlerFunc(pattern string) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			// Validate request payloads
			bufBody := bytes.Buffer{}
			if _, err := bufBody.ReadFrom(r.Body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if err := verifySignature(r.Header.Get("X-Hub-Signature"), ws.Secret, bufBody.Bytes()); err != nil {
				w.WriteHeader(http.StatusForbidden)
				return
			}

			// Parse payloads
			reqBody := decode.RequestBody{}
			if err := json.Unmarshal(bufBody.Bytes(), &reqBody); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			// Switch handler by pattern and object
			objectHandlerMap, exist := ws.objectHandlers[pattern]
			if !exist {
				// if pattern not registered, return ok status.
				w.WriteHeader(http.StatusOK)
				return
			}
			handler, exist := objectHandlerMap[reqBody.Object]
			if !exist {
				// if object handler not registered, return ok status.
				w.WriteHeader(http.StatusOK)
				return
			}
			if err := handler(reqBody); err != nil {
				w.WriteHeader(http.StatusBadRequest)
			} else {
				w.WriteHeader(http.StatusOK)
			}
		default:
			w.WriteHeader(http.StatusForbidden)
		}
		return
	})
}

func verifySignature(sig, secret string, payload []byte) error {
	if sig == "" {
		return errors.New("error: signature is empty")
	}

	elements := strings.Split(sig, "=")
	if len(elements) < 2 {
		return errors.New("errors: invalid signature")
	}
	signatureHash := elements[1]

	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write(payload)
	expectedHash := hex.EncodeToString(mac.Sum(nil))

	if signatureHash != expectedHash {
		return errors.New("error: signature hash do not match expected hash")
	}
	return nil
}
