package service

import (
	"encoding/binary"
	"github.com/danieldin95/openlan-go/libol"
	"github.com/danieldin95/openlan-go/models"
	"net"
	"sync"
)

type _network struct {
	lock sync.RWMutex
	networks  map[string]*models.Network
	usedaddr  map[string]bool
}

var Network = _network {
	networks: make(map[string]*models.Network, 1024),
	usedaddr: make(map[string]bool, 1024),
}

func (w *_network) Load(path string) error {
	nets := make([]*models.Network, 32)

	if err := libol.UnmarshalLoad(&nets, path); err != nil {
		libol.Error("_network.load: %s", err)
		return err
	}

	for _, net := range nets {
		libol.Info("%s", net)
		w.networks[net.Tenant] = net
	}

	return nil
}

func (w *_network) AddNet(n *models.Network) {
	w.lock.Lock()
	defer w.lock.Unlock()

	w.networks[n.Tenant] = n
	//TODO save to db.
}

func (w *_network) DelNet(name string) {
	w.lock.Lock()
	defer w.lock.Unlock()

	if _, ok := w.networks[name]; ok {
		delete(w.networks, name)
	}
}

func (w *_network) GetNet(name string) *models.Network {
	w.lock.RLock()
	defer w.lock.RUnlock()

	if u, ok := w.networks[name]; ok {
		return u
	}

	return nil
}

//TODO add/del route

func (w *_network) ListNet() <-chan *models.Network {
	c := make(chan *models.Network, 128)

	go func() {
		w.lock.RLock()
		defer w.lock.RUnlock()

		for _, u := range w.networks {
			c <- u
		}
		c <- nil //Finish channel by nil.
	}()

	return c
}

func (w *_network) GetFreeAddr(n *models.Network) (string, string) {
	w.lock.Lock()
	defer w.lock.Unlock()

	ipStr  := ""
	netmask := n.Netmask
	ip := net.ParseIP(n.IpAddr).To4()
	start := binary.BigEndian.Uint32(ip[:4])

	for i := 0; i < n.IpRange; i++ {
		tmp := make([]byte, 4)
		binary.BigEndian.PutUint32(tmp[:4], start)
		
		tmpStr := net.IP(tmp).String()
		if ok, _ := w.usedaddr[tmpStr]; !ok {
			ipStr = tmpStr
			break
		}

		start += 1
	}

	if ipStr != "" {
		w.usedaddr[ipStr] = true
	}

	return ipStr, netmask
}

