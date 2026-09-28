package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"agentpal/internal/central"
	"agentpal/internal/codex"
	"agentpal/internal/constants"
	"agentpal/internal/types"
)

func main() {
	port := flag.Int("port", constants.DefaultPort, "HTTP listen port")
	codexDir := flag.String("codex-dir", "~/.codex", "published Codex directory")
	backupDir := flag.String("backup-dir", "~/.agentpal/server/auth-backups", "authentication backup directory")
	adminIssuer := flag.String("admin-jwt-issuer", "internal-sso-portal", "trusted Portal JWT issuer")
	adminPublicKey := flag.String("admin-jwt-public-key", "", "Portal JWT public key PEM path")
	portalLoginURL := flag.String("portal-login-url", "https://a.yprenewables.com/login", "Portal login URL")
	casdoorURL := flag.String("casdoor-url", "https://id.yprenew.com", "Casdoor base URL")
	casdoorClientID := flag.String("casdoor-client-id", "", "Casdoor application client ID")
	casdoorScopes := flag.String("casdoor-scopes", "openid profile", "Casdoor authorization scopes")
	oauthRedirectURL := flag.String("oauth-redirect-url", "", "OAuth redirect URL registered on the Casdoor application")
	userStoreDir := flag.String("user-store-dir", "~/.agentpal/server/users", "central user reserved profile directory")
	flag.Parse()

	expandedCodexDir, err := codex.ExpandPath(*codexDir)
	if err != nil {
		log.Fatal(err)
	}
	expandedBackupDir, err := codex.ExpandPath(*backupDir)
	if err != nil {
		log.Fatal(err)
	}
	expandedUserStoreDir, err := codex.ExpandPath(*userStoreDir)
	if err != nil {
		log.Fatal(err)
	}
	server := central.NewServer(types.ServerOptions{
		CodexDir:         expandedCodexDir,
		Port:             *port,
		AdminJWTIssuer:   *adminIssuer,
		AdminJWTPublic:   *adminPublicKey,
		PortalLoginURL:   *portalLoginURL,
		CasdoorURL:       *casdoorURL,
		CasdoorClientID:  *casdoorClientID,
		CasdoorScopes:    *casdoorScopes,
		OAuthRedirectURL: *oauthRedirectURL,
		UserStoreDir:     expandedUserStoreDir,
	})
	server.BackupDir = expandedBackupDir
	if err := server.Start(); err != nil {
		log.Fatal(err)
	}
	log.Printf("%s central node listening on :%d", constants.AppName, *port)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	<-signals
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Stop(ctx); err != nil {
		log.Printf("central node shutdown: %v", err)
	}
}
