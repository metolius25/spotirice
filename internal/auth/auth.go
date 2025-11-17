package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"os/exec"

	"github.com/metolius25/spotirice/internal/config"
	spotify "github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2"
)

const redirectURI = "http://127.0.0.1:8000/callback"

func Authenticate() (*spotify.Client, error) {
	creds, err := config.LoadCredentials()
	if err != nil {
		return nil, fmt.Errorf("could not load credentials: %w", err)
	}

	auth := spotifyauth.New(
		spotifyauth.WithRedirectURL(redirectURI),
		spotifyauth.WithScopes(
			spotifyauth.ScopeUserReadPrivate,
			spotifyauth.ScopeUserReadPlaybackState,
            spotifyauth.ScopeUserReadCurrentlyPlaying,
			spotifyauth.ScopeUserModifyPlaybackState,
            spotifyauth.ScopeUserLibraryRead,
            spotifyauth.ScopeUserLibraryModify,

		),
		spotifyauth.WithClientID(creds.ClientID),
		spotifyauth.WithClientSecret(creds.ClientSecret),
	)

	if config.TokenExists() {
		token, err := config.LoadToken()
		if err == nil {
			client := spotify.New(auth.Client(context.Background(), token))
			return client, nil
		}
		log.Printf("Could not load token, re-authenticating: %v", err)
	}

	return fullOAuthFlow(auth)
}

func fullOAuthFlow(auth *spotifyauth.Authenticator) (*spotify.Client, error) {
	state, err := generateRandomState()
	if err != nil {
		return nil, err
	}

	ch := make(chan *oauth2.Token)
	errCh := make(chan error)

	mux := http.NewServeMux()
	server := &http.Server{Addr: "127.0.0.1:8000", Handler: mux}

	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		token, err := auth.Token(r.Context(), state, r)
		if err != nil {
			log.Printf("Error getting token: %v", err)
			http.Error(w, "Couldn't get token", http.StatusForbidden)
			errCh <- fmt.Errorf("couldn't get token: %w", err)
			return
		}
		if err := config.SaveToken(token); err != nil {
			log.Printf("Could not save token: %v", err)
		}
		fmt.Fprintln(w, "Authenticated! You can close this window.")
		ch <- token
	})

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("server failed: %w", err)
		}
	}()
	defer server.Shutdown(context.Background())

	url := auth.AuthURL(state)
	fmt.Println("Please log in to Spotify by visiting the following page in your browser:", url)
	exec.Command("xdg-open", url).Start()

	select {
	case token := <-ch:
		client := spotify.New(auth.Client(context.Background(), token))
		return client, nil
	case err := <-errCh:
		return nil, err
	}
}

func generateRandomState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("could not generate random state: %w", err)
	}
	return base64.URLEncoding.EncodeToString(b), nil
}