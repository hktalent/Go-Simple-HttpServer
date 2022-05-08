package main

import (
	"errors"
	"github.com/fsnotify/fsnotify"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"regexp"
	"strconv"
)

// bind server config
// AccessPath default is  StaticDir
type OneServerBindConfig struct {
	Name       string `json:"name,omitempty"`
	StaticDir  string `json:"staticDir,omitempty"`
	AccessPath string `json:"accessPath,omitempty"`
	BindIp     string `json:"bindIp,omitempty"`
	BindPort   int    `json:"bindPort,omitempty"`
	Url        string `json:"url,omitempty"`
	Des        string `json:"des,omitempty"`
}

// Server config
type Configs struct {
	BindIp   string                `json:"bindIp,omitempty"`
	BindPort int                   `json:"bindPort,omitempty"`
	Servers  []OneServerBindConfig `jsong:"servers,omitempty"`
	Verbose  bool                  `json:"verbose,omitempty"`
}

// default Bind Conf
var defaultBindConf Configs = Configs{
	BindIp:   "0.0.0.0",
	BindPort: 8082,
	Verbose:  true}

func fnLog(x interface{}) {
	if defaultBindConf.Verbose {
		log.Println(x)
	}
}

// init config
func fnInit() {
	viper.AddConfigPath("./config")
	viper.AddConfigPath("$HOME")
	err := viper.ReadInConfig() // 查找并读取配置文件
	if err != nil {             // 处理读取配置文件的错误
		fnLog(err)
		return
	}
	// 将读取的配置信息保存至全局变量Conf
	if err := viper.Unmarshal(&defaultBindConf); err != nil {
		fnLog(err)
		return
	}
	// 监控配置文件变化
	viper.WatchConfig()
	viper.OnConfigChange(func(in fsnotify.Event) {
		if err := viper.Unmarshal(&defaultBindConf); err != nil {
			fnLog(err)
		} else {
			// 重启服务
		}
	})
}

// get free port
func GetFreePort() (port int, err error) {
	var a *net.TCPAddr
	if a, err = net.ResolveTCPAddr("tcp", "localhost:0"); err == nil {
		var l *net.TCPListener
		if l, err = net.ListenTCP("tcp", a); err == nil {
			defer l.Close()
			return l.Addr().(*net.TCPAddr).Port, nil
		}
	}
	return
}

// 反向代理封装
func DoReverseProxy(c *gin.Context, target string) {
	remote, err := url.Parse(target)
	if nil == err {
		director := func(req *http.Request) {
			req.Header = c.Request.Header
			req.URL.Scheme = remote.Scheme
			req.Host = remote.Host
			req.URL.Host = remote.Host
			req.RequestURI = c.Request.RequestURI
			req.URL.Path = c.Request.RequestURI
		}
		proxy := &httputil.ReverseProxy{Director: director}
		proxy.ServeHTTP(c.Writer, c.Request)
	}
}

// 反向代理分装，最后必须是类似：*id
func ReverseProxy(path, target string, router *gin.Engine) {
	xxx := func(c *gin.Context) {
		DoReverseProxy(c, target)
	}
	//router.Group(path, xxx)
	router.GET(path, xxx)
	router.POST(path, xxx)
}

func DoConfig(config OneServerBindConfig) OneServerBindConfig {
	if 0 == config.BindPort {
		if port, err := GetFreePort(); err == nil {
			config.BindPort = port
		}
	}
	if "" == config.BindIp {
		config.BindIp = "127.0.0.1"
	}
	if "" == config.AccessPath && "" != config.StaticDir {
		if xr, err := regexp.Compile(`.*\/`); err == nil {
			config.AccessPath = "/" + string(xr.ReplaceAll([]byte(config.StaticDir), []byte("")))
		}
	}
	return config
}

func StartServer(config *OneServerBindConfig) *gin.Engine {
	// 有url表示已经有服务，只做服务转发，不启动server
	if "" == config.Url {
		config.Url = "http://127.0.0.1:" + strconv.Itoa(config.BindPort)
		//spew.Println("xxxx: " + config.StaticDir)
		router := gin.Default()
		if "" != config.StaticDir {
			router.Use(static.Serve("/", static.LocalFile(config.StaticDir, false)))
			s1 := config.StaticDir + "/index.html"
			if xr, err := regexp.Compile(`\/*`); err != nil {
				s1 = string(xr.ReplaceAll([]byte(s1), []byte("/")))
			}
			//s1 = strings.Replace(s1, "//", "/", -1)
			if _, err := os.Stat(s1); !errors.Is(err, os.ErrNotExist) {
				//fnLog(s1)
				router.NoRoute(func(c *gin.Context) {
					c.File(s1)
				})
			}
		}
		szServer := config.BindIp + ":" + strconv.Itoa(config.BindPort)
		go router.Run(szServer)
		return router
	}
	return nil
}

func main() {
	fnInit()
	if !defaultBindConf.Verbose {
		gin.SetMode(gin.ReleaseMode)
	}
	router := StartServer(&OneServerBindConfig{BindIp: defaultBindConf.BindIp, BindPort: defaultBindConf.BindPort})
	if nil != router {
		router.SetTrustedProxies([]string{"127.0.0.1"})
		for _, x := range defaultBindConf.Servers {
			x = DoConfig(x)
			StartServer(&x)
			if !("" == x.AccessPath || "/" == x.AccessPath) {
				//spew.Println("[[[[ " + x.AccessPath)
				ReverseProxy(x.AccessPath, x.Url, router)
			}
		}
	}
	select {}
}
