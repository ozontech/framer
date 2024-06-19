package flowcontrol

import (
	"sync"
)

type FlowControl struct {
	n    uint32
	cond *sync.Cond
	ok   bool
}

func NewFlowControl(n uint32) *FlowControl {
	fc := FlowControl{
		n:    n,
		cond: sync.NewCond(&sync.Mutex{}),
		ok:   true,
	}
	return &fc
}

func (fc *FlowControl) Wait(n uint32) bool {
	if n == 0 {
		return true
	}
	cond := fc.cond

	cond.L.Lock()
	defer cond.L.Unlock()

	for n > fc.n && fc.ok {
		cond.Wait()
	}
	fc.n -= n
	return fc.ok
}

func (fc *FlowControl) Add(n uint32) {
	fc.cond.L.Lock()
	defer fc.cond.L.Unlock()

	fc.n += n
	fc.cond.Broadcast() // оповещаем все горутины, заблокированный в ожидании flowControl проверить лимиты
}

func (fc *FlowControl) Reset(n uint32) {
	// тут лок нужен чтобы избежать ситуации гонок, когда ошибку уже установили,
	// но Wait еще не вернул результат
	fc.cond.L.Lock()
	defer fc.cond.L.Unlock()

	fc.n = n
	fc.ok = true
}

func (fc *FlowControl) Disable() {
	fc.cond.L.Lock()
	defer fc.cond.L.Unlock()

	fc.ok = false
	fc.cond.Broadcast()
}
