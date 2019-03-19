package routes

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jamesnetherton/m3u"

	"github.com/pierre-emmanuelJ/iptv-proxy/pkg/config"
	proxyM3U "github.com/pierre-emmanuelJ/iptv-proxy/pkg/m3u"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

const maxForbiddenRestart = 10

type proxy struct {
	*config.ProxyConfig
	*m3u.Track
	newM3U []byte
}

// Serve the pfinder api
func Serve(proxyConfig *config.ProxyConfig) error {
	router := gin.Default()
	router.Use(cors.Default())
	newM3U, err := initm3u(proxyConfig)
	if err != nil {
		return err
	}
	Routes(proxyConfig, router.Group("/"), newM3U)

	return router.Run(fmt.Sprintf(":%d", proxyConfig.HostConfig.Port))
}

// Routes adds the routes for the app to the RouterGroup r
func Routes(proxyConfig *config.ProxyConfig, r *gin.RouterGroup, newM3U []byte) {

	p := &proxy{
		proxyConfig,
		nil,
		newM3U,
	}

	r.GET("/iptv.m3u", p.authenticate, p.getM3U)

	// XXX Private need for external Android app
	r.POST("/iptv.m3u", p.authenticate, p.getM3U)

	//IPTV Smarter android app compatibility
	r.POST("/player_api.php", p.iptvSmarterAPP)
	r.GET("/player_api.php", p.iptvSmarterAPP)
	r.POST("/xmltv.php", p.iptvSmarterAPP)
	r.GET("/xmltv.php", p.iptvSmarterAPP)

	for i, track := range proxyConfig.Playlist.Tracks {
		oriURL, err := url.Parse(track.URI)
		if err != nil {
			return
		}
		tmp := &proxy{
			nil,
			&proxyConfig.Playlist.Tracks[i],
			nil,
		}
		r.GET(oriURL.RequestURI(), p.authenticate, tmp.reverseProxy)
	}
}

func (p *proxy) iptvSmarterAPP(c *gin.Context) {
	remoteHostURL := p.ProxyConfig.RemoteURL
	var remoteHost string
	if remoteHostURL.Port() != "" {
		remoteHost = fmt.Sprintf("%s:%s", remoteHostURL.Hostname(), remoteHostURL.Port())
	} else {
		remoteHost = remoteHostURL.Hostname()
	}

	req := c.Request

	req.Host = remoteHostURL.Hostname()
	newURL, err := url.Parse(fmt.Sprintf("http://%s%s", remoteHost, req.URL.RequestURI()))
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	req.URL = newURL
	req.RequestURI = ""

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	defer resp.Body.Close()

	copyHTTPHeader(c, resp.Header)
	c.Stream(func(w io.Writer) bool {
		io.Copy(w, resp.Body)
		return false
	})
}

func (p *proxy) reverseProxy(c *gin.Context) {

	log.Printf("[iptv-proxy] %v | %s |Track\t%s\n",
		time.Now().Format("2006/01/02 - 15:04:05"),
		c.ClientIP(), p.Track.Name,
	)

	rpURL, err := url.Parse(p.Track.URI)
	if err != nil {
		log.Fatal(err)
	}

	forbiddenRestart := maxForbiddenRestart
	c.Stream(func(w io.Writer) bool {
		resp, err := http.Get(rpURL.String())
		if err != nil {
			c.AbortWithError(http.StatusInternalServerError, err)
			return false
		}
		defer resp.Body.Close()

		c.Status(resp.StatusCode)

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusForbidden {
			return false
		}

		defer log.Printf("[iptv-proxy] %v | %d | %s |Restart track\t%s\n",
			time.Now().Format("2006/01/02 - 15:04:05"),
			resp.StatusCode,
			c.ClientIP(), p.Track.Name,
		)

		if resp.StatusCode == http.StatusForbidden && forbiddenRestart > 0 {
			forbiddenRestart--
			return true
		}

		copyHTTPHeader(c, resp.Header)
		io.Copy(w, resp.Body)

		return true
	})
}

func copyHTTPHeader(c *gin.Context, header http.Header) {
	for k, v := range header {
		c.Header(k, strings.Join(v, ", "))
	}
}

func (p *proxy) getM3U(c *gin.Context) {
	c.Header("Content-Disposition", "attachment; filename=\"iptv.m3u\"")
	c.Data(http.StatusOK, "application/octet-stream", p.newM3U)
}

// AuthRequest handle auth credentials
type AuthRequest struct {
	User     string `form:"user" binding:"required"`
	Password string `form:"password" binding:"required"`
} // XXX very unsafe

func (p *proxy) authenticate(ctx *gin.Context) {
	var authReq AuthRequest
	if err := ctx.Bind(&authReq); err != nil {
		ctx.AbortWithError(http.StatusBadRequest, err)
		return
	}
	//XXX very unsafe
	if p.ProxyConfig.User != authReq.User || p.ProxyConfig.Password != authReq.Password {
		ctx.AbortWithStatus(http.StatusUnauthorized)
	}
}

func initm3u(p *config.ProxyConfig) ([]byte, error) {
	playlist, err := proxyM3U.ReplaceURL(p)
	if err != nil {
		return nil, err
	}

	result, err := proxyM3U.Marshall(playlist)
	if err != nil {
		return nil, err
	}

	return []byte(result), nil
}
