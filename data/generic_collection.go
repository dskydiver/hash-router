package data

import (
	"sync"

	"gitlab.com/TitanInd/hashrouter/interfaces"
)

type Collection[T interfaces.IModel] struct {
	items sync.Map
}

func NewCollection[T interfaces.IModel]() *Collection[T] {
	return &Collection[T]{
		items: sync.Map{},
	}
}

func (p *Collection[T]) Load(ID string) (item T, ok bool) {
	if val, ok := p.items.Load(ID); ok {
		return val.(T), true
	} else {
		if val != nil {
			return val.(T), false
		}
		return *new(T), false
	}
}

func (p *Collection[T]) Range(f func(item T) bool) {
	p.items.Range(func(key, value any) bool {
		item := value.(T)
		return f(item)
	})
}

func (p *Collection[T]) Store(item T) {
	p.items.Store(item.GetID(), item)
}

func (p *Collection[T]) Delete(ID string) {
	p.items.Delete(ID)
}
