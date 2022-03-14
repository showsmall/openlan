package olsw

import (
	co "github.com/danieldin95/openlan/pkg/config"
	"github.com/danieldin95/openlan/pkg/network"
	"github.com/danieldin95/openlan/pkg/olsw/api"
)

type Networker interface {
	String() string
	ID() string
	Initialize()
	Start(v api.Switcher)
	Stop()
	GetBridge() network.Bridger
	GetConfig() *co.Network
	GetSubnet() string
	Reload(c *co.Network)
}

var Workers = make(map[string]Networker)

func NewNetworker(c *co.Network) Networker {
	var obj Networker
	switch c.Provider {
	case "esp":
		obj = NewESPWorker(c)
	case "vxlan":
		obj = NewVxLANWorker(c)
	case "fabric":
		obj = NewFabricWorker(c)
	default:
		obj = NewOpenLANWorker(c)
	}
	Workers[c.Name] = obj
	return obj
}
