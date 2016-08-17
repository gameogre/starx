package starx

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/chrislonng/starx/cluster"
	"github.com/chrislonng/starx/log"
	"github.com/chrislonng/starx/network"
	"golang.org/x/net/websocket"
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
	log.Infof("loading %s settings", App.Config.Type)
	if setting, ok := settings[App.Config.Type]; ok && len(setting) > 0 {
		for _, fn := range setting {
			fn()
		}
	}
}

func welcomeMsg() {
	fmt.Println(asciiLogo)
}

func (app *starxApp) init() {
	// get server id from command line
	var serverId string
	cl := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	cl.StringVar(&serverId, "server-id", "", "server id")
	cl.SetOutput(ioutil.Discard)
	cl.Parse(os.Args[1:])

	// init
	if App.Standalone {
		if strings.TrimSpace(serverId) == "" {
			log.Fatalf("server running in standalone mode, but not found server id argument")
			os.Exit(-1)
		}

		cfg, err := cluster.Server(serverId)
		if err != nil {
			log.Fatalf(err.Error())
			os.Exit(-1)
		}

		App.Config = cfg
	} else {
		// if server running in cluster mode, master server config require
		// initialize master server config
		if !fileExist(masterConfigPath) {
			log.Fatalf("%s not found", masterConfigPath)
			os.Exit(-1)
		} else {
			f, _ := os.Open(masterConfigPath)
			defer f.Close()

			reader := json.NewDecoder(f)
			var master *cluster.ServerConfig
			for {
				if err := reader.Decode(master); err == io.EOF {
					break
				} else if err != nil {
					log.Errorf(err.Error())
				}
			}

			master.Type = "master"
			master.IsMaster = true
			App.Master = master
			cluster.Register(master)
		}
		if App.Master == nil {
			log.Fatalf("wrong master server config file(%s)", masterConfigPath)
			os.Exit(-1)
		}

		if strings.TrimSpace(serverId) == "" {
			// not pass server id, running in master mode
			App.Config = App.Master
		} else {
			cfg, err := cluster.Server(serverId)
			if err != nil {
				log.Fatalf(err.Error())
				os.Exit(-1)
			}

			App.Config = cfg
		}
	}

	// dependencies initialization
	network.SetAppConfig(App.Config)
	cluster.SetAppConfig(App.Config)
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
		log.Infof("The app will shutdown in a few seconds")
	case s := <-sg:
		log.Infof("Got signal: %v", s)
	}
	log.Infof("server: " + app.Config.Id + " is stopping...")
	network.Shutdown()
	close(endRunning)
}

// Enable current server accept connection
func (app *starxApp) listenAndServe() {
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", app.Config.Host, app.Config.Port))
	if err != nil {
		log.Errorf(err.Error())
		os.Exit(-1)
	}
	log.Infof("listen at %s:%d(%s)",
		app.Config.Host,
		app.Config.Port,
		app.Config.String())

	defer listener.Close()
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Errorf(err.Error())
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

	log.Infof("listen at %s:%d(%s)",
		app.Config.Host,
		app.Config.Port,
		app.Config.String())

	err := http.ListenAndServe(fmt.Sprintf("%s:%d", app.Config.Host, app.Config.Port), nil)

	if err != nil {
		panic("ListenAndServe: " + err.Error())
	}
}
