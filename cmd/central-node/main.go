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
	flag.Parse()

	expandedCodexDir, err := codex.ExpandPath(*codexDir)
	if err != nil {
		log.Fatal(err)
	}
	expandedBackupDir, err := codex.ExpandPath(*backupDir)
	if err != nil {
		log.Fatal(err)
	}
	server := central.NewServer(types.ServerOptions{
		CodexDir:       expandedCodexDir,
		Port:           *port,
		AdminJWTIssuer: *adminIssuer,
		AdminJWTPublic: *adminPublicKey,
		PortalLoginURL: *portalLoginURL,
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
