package swagger

import (
	"net/http"

	"github.com/gin-gonic/gin"
	swaggerUi "github.com/go-swagno/swagno-files"
	"golang.org/x/net/webdav"
)

type Config struct {
	Prefix string
}

var swaggerDoc string

var handler *webdav.Handler

var defaultConfig = Config{
	Prefix: "/swagger",
}

func SwaggerHandler(doc []byte, config ...Config) gin.HandlerFunc {
	if len(config) != 0 {
		defaultConfig = config[0]
	}
	if swaggerDoc == "" {
		swaggerDoc = string(doc)
	}
	if handler == nil {
		handler = swaggerUi.Handler
	}

	return func(ctx *gin.Context) {
		prefix := defaultConfig.Prefix
		handler.Prefix = prefix

		switch ctx.Request.RequestURI {
		case prefix + "/":
			ctx.Redirect(http.StatusMovedPermanently, "/swagger/index.html")
		case prefix + "/doc.json":
			ctx.String(http.StatusOK, swaggerDoc)
		default:
			handler.ServeHTTP(ctx.Writer, ctx.Request)
		}
	}
}
