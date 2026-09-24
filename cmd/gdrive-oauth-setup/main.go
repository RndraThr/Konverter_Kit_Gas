// gdrive-oauth-setup is a one-time, interactive tool that authorizes Konkit
// to write to a specific Google account's Drive using OAuth2 (not a Service
// Account — Service Accounts have no storage quota of their own and cannot
// upload files, even to a folder shared with them).
//
// Run it locally, on a machine with a real browser:
//
//	go run ./cmd/gdrive-oauth-setup -client-id=... -client-secret=...
//
// It opens (or prints) a Google consent URL, catches the redirect on a
// temporary local server, exchanges the code for a token, creates a fresh
// Drive folder owned by the authorizing account (so it's visible under the
// narrow drive.file scope), and writes the token to a JSON file. Both the
// token file path and the printed folder ID then go into the server's
// GDRIVE_OAUTH_TOKEN_JSON / GDRIVE_ROOT_FOLDER_ID configuration.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

const callbackPort = "8085"

func main() {
	clientID := flag.String("client-id", "", "OAuth 2.0 Client ID (from Google Cloud Console)")
	clientSecret := flag.String("client-secret", "", "OAuth 2.0 Client Secret")
	outPath := flag.String("out", "gdrive-oauth-token.json", "where to write the resulting token JSON")
	folderName := flag.String("folder-name", "Konkit", "name of the Drive folder to create as the storage root")
	flag.Parse()

	if *clientID == "" || *clientSecret == "" {
		log.Fatal("usage: go run ./cmd/gdrive-oauth-setup -client-id=... -client-secret=...")
	}

	config := &oauth2.Config{
		ClientID:     *clientID,
		ClientSecret: *clientSecret,
		Endpoint:     google.Endpoint,
		RedirectURL:  "http://localhost:" + callbackPort + "/callback",
		Scopes:       []string{drive.DriveFileScope},
	}

	code, err := awaitAuthorizationCode(config)
	if err != nil {
		log.Fatalf("authorization failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	token, err := config.Exchange(ctx, code)
	if err != nil {
		log.Fatalf("exchange code for token: %v", err)
	}

	tokenBytes, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		log.Fatalf("marshal token: %v", err)
	}
	if err := os.WriteFile(*outPath, tokenBytes, 0o600); err != nil {
		log.Fatalf("write token file %s: %v", *outPath, err)
	}
	fmt.Printf("Token saved to %s\n", *outPath)

	service, err := drive.NewService(ctx, option.WithTokenSource(config.TokenSource(ctx, token)))
	if err != nil {
		log.Fatalf("create drive service: %v", err)
	}
	folder, err := service.Files.Create(&drive.File{
		Name:     *folderName,
		MimeType: "application/vnd.google-apps.folder",
	}).Context(ctx).Do()
	if err != nil {
		log.Fatalf("create root folder %q: %v", *folderName, err)
	}

	fmt.Printf("\nSetup complete.\n")
	fmt.Printf("  GDRIVE_OAUTH_TOKEN_JSON should point at: %s\n", *outPath)
	fmt.Printf("  GDRIVE_ROOT_FOLDER_ID=%s\n", folder.Id)
}

// awaitAuthorizationCode prints the consent URL, starts a temporary local
// server to catch Google's redirect, and returns the authorization code
// once the user has approved access in their browser.
func awaitAuthorizationCode(config *oauth2.Config) (string, error) {
	state := randomState()
	authURL := config.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)

	fmt.Println("Open this URL in a browser and approve access:")
	fmt.Println()
	fmt.Println(authURL)
	fmt.Println()
	fmt.Println("Waiting for authorization...")

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	mux := http.NewServeMux()
	server := &http.Server{Addr: ":" + callbackPort, Handler: mux}

	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			errCh <- fmt.Errorf("state mismatch in callback")
			return
		}
		if errParam := r.URL.Query().Get("error"); errParam != "" {
			http.Error(w, "authorization denied", http.StatusBadRequest)
			errCh <- fmt.Errorf("google returned error: %s", errParam)
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "missing code", http.StatusBadRequest)
			errCh <- fmt.Errorf("callback had no code parameter")
			return
		}
		fmt.Fprintln(w, "Authorization complete — you can close this tab and return to the terminal.")
		codeCh <- code
	})

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("callback server: %w", err)
		}
	}()

	var code string
	var resultErr error
	select {
	case code = <-codeCh:
	case resultErr = <-errCh:
	case <-time.After(5 * time.Minute):
		resultErr = fmt.Errorf("timed out waiting for authorization")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)

	return code, resultErr
}

func randomState() string {
	return fmt.Sprintf("konkit-%d", time.Now().UnixNano())
}
