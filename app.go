package starx

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/net/websocket"

	"github.com/chrislonng/starx/cluster"
	"github.com/chrislonng/starx/log"
	"github.com/chrislonng/starx/network"
)

type starxApp struct {
	Master     *cluster.ServerConfig // master server config
	Config     *cluster.ServerConfig // current server information
	AppName    string
	Standalone bool // current server is running in standalone mode
	StartTime  time.Time
}

func newApp() *starxApp {
	return &starxApp{StartTime: time.Now()}
}

func loadSettings() {
	log.Info("loading %s settings", App.Config.Type)
	if setting, ok := settings[App.Config.Type]; ok && len(setting) > 0 {
		for _, fn := range setting {
			fn()
		}
	}
}

func welcomeMsg() {
	fmt.Println(asciiLogo)
}

func (app *starxApp) start() {
	network.Startup()

	if app.Config.IsWebsocket {
		app.listenAndServeWS()
	} else {
		app.listenAndServe()
	}

	sg := make(chan os.Signal, 1)
	signal.Notify(sg, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	// stop server
	select {
	case <-endRunning:
		log.Info("The app will shutdown in a few seconds")
	case s := <-sg:
		log.Info("Got signal: %v", s)
	}
	log.Info("server: " + app.Config.Id + " is stopping...")
	network.Shutdown()
	close(endRunning)
}

// Enable current server accept connection
func (app *starxApp) listenAndServe() {
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", app.Config.Host, app.Config.Port))
	if err != nil {
		log.Error(err.Error())
		os.Exit(-1)
	}
	log.Info("listen at %s:%d(%s)",
		app.Config.Host,
		app.Config.Port,
		app.Config.String())

	defer listener.Close()
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Error(err.Error())
			continue
		}
		if app.Config.IsFrontend {
			go network.Handler.Handle(conn)
		} else {
			go network.Remote.Handle(conn)
		}
	}
}

func (app *starxApp) listenAndServeWS() {
	http.Handle("/", websocket.Handler(network.Handler.HandleWS))

	log.Info("listen at %s:%d(%s)",
		app.Config.Host,
		app.Config.Port,
		app.Config.String())

	err := http.ListenAndServe(fmt.Sprintf("%s:%d", app.Config.Host, app.Config.Port), nil)

	if err != nil {
		panic("ListenAndServe: " + err.Error())
	}
}
