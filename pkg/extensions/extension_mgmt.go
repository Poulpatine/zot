//go:build mgmt
// +build mgmt

package extensions

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"

	"zotregistry.io/zot/pkg/api/config"
	"zotregistry.io/zot/pkg/api/constants"
	zcommon "zotregistry.io/zot/pkg/common"
	"zotregistry.io/zot/pkg/log"
)

type HTPasswd struct {
	Path string `json:"path,omitempty"`
}

type BearerConfig struct {
	Realm   string `json:"realm,omitempty"`
	Service string `json:"service,omitempty"`
}

type Auth struct {
	HTPasswd *HTPasswd     `json:"htpasswd,omitempty" mapstructure:"htpasswd"`
	Bearer   *BearerConfig `json:"bearer,omitempty" mapstructure:"bearer"`
	LDAP     *struct {
		Address string `json:"address,omitempty" mapstructure:"address"`
	} `json:"ldap,omitempty" mapstructure:"ldap"`
}

type StrippedConfig struct {
	DistSpecVersion string `json:"distSpecVersion" mapstructure:"distSpecVersion"`
	BinaryType      string `json:"binaryType" mapstructure:"binaryType"`
	HTTP            struct {
		Auth *Auth `json:"auth,omitempty" mapstructure:"auth"`
	} `json:"http" mapstructure:"http"`
}

func IsBuiltWithMGMTExtension() bool {
	return true
}

func (auth Auth) MarshalJSON() ([]byte, error) {
	type localAuth Auth

	if auth.Bearer == nil && auth.LDAP == nil &&
		auth.HTPasswd.Path == "" {
		auth.HTPasswd = nil

		return json.Marshal((localAuth)(auth))
	}

	if auth.HTPasswd.Path == "" && auth.LDAP == nil {
		auth.HTPasswd = nil
	} else {
		auth.HTPasswd.Path = ""
	}

	auth.LDAP = nil

	return json.Marshal((localAuth)(auth))
}

type mgmt struct {
	config *config.Config
	log    log.Logger
}

// mgmtHandler godoc
// @Summary Get current server configuration
// @Description Get current server configuration
// @Router 	/v2/_zot/ext/mgmt [get]
// @Accept  json
// @Produce json
// @Success 200 {object} 	extensions.StrippedConfig
// @Failure 500 {string} 	string 				"internal server error".
func (mgmt *mgmt) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sanitizedConfig := mgmt.config.Sanitize()
		buf, err := zcommon.MarshalThroughStruct(sanitizedConfig, &StrippedConfig{})
		if err != nil {
			mgmt.log.Error().Err(err).Msg("mgmt: couldn't marshal config response")
			w.WriteHeader(http.StatusInternalServerError)
		}
		_, _ = w.Write(buf)
	})
}

func SetupMgmtRoutes(config *config.Config, router *mux.Router, log log.Logger) {
	if config.Extensions.Mgmt != nil && *config.Extensions.Mgmt.Enable {
		log.Info().Msg("setting up mgmt routes")

		mgmt := mgmt{config: config, log: log}

		allowedMethods := zcommon.AllowedMethods(http.MethodGet)

		mgmtRouter := router.PathPrefix(constants.ExtMgmt).Subrouter()
		mgmtRouter.Use(zcommon.ACHeadersHandler(allowedMethods...))
		mgmtRouter.Use(zcommon.AddExtensionSecurityHeaders())
		mgmtRouter.Methods(allowedMethods...).Handler(mgmt.handler())
	}
}
