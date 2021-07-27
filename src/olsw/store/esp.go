package store

import (
	"github.com/danieldin95/openlan-go/src/libol"
	"github.com/danieldin95/openlan-go/src/models"
)

type esp struct {
	Esp *libol.SafeStrMap
}

func (p *esp) Init(size int) {
	p.Esp = libol.NewSafeStrMap(size)
}

func (p *esp) Add(esp *models.Esp) {
	_ = p.Esp.Set(esp.ID(), esp)
}

func (p *esp) Get(key string) *models.Esp {
	ret := p.Esp.Get(key)
	if ret != nil {
		return ret.(*models.Esp)
	}
	return nil
}

func (p *esp) Del(key string) {
	p.Esp.Del(key)
}

func (p *esp) List() <-chan *models.Esp {
	c := make(chan *models.Esp, 128)
	go func() {
		p.Esp.Iter(func(k string, v interface{}) {
			m := v.(*models.Esp)
			m.Update()
			c <- m
		})
		c <- nil //Finish channel by nil.
	}()
	return c
}

var Esp = esp{
	Esp: libol.NewSafeStrMap(1024),
}

type espState struct {
	State *libol.SafeStrMap
}

func (p *espState) Init(size int) {
	p.State = libol.NewSafeStrMap(size)
}

func (p *espState) Add(esp *models.EspState) {
	_ = p.State.Set(esp.ID(), esp)
}

func (p *espState) Get(key string) *models.EspState {
	ret := p.State.Get(key)
	if ret != nil {
		return ret.(*models.EspState)
	}
	return nil
}

func (p *espState) Del(key string) {
	p.State.Del(key)
}

func (p *espState) List() <-chan *models.EspState {
	c := make(chan *models.EspState, 128)
	go func() {
		p.State.Iter(func(k string, v interface{}) {
			m := v.(*models.EspState)
			m.Update()
			c <- m
		})
		c <- nil //Finish channel by nil.
	}()
	return c
}

var EspState = espState{
	State: libol.NewSafeStrMap(1024),
}
