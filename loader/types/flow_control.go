package types

type FlowControl interface {
	Wait(n uint32) (ok bool) // Ждем разрешения на отправку пакета длиной n. Если ok == false, стрим больше не может отправлять данные
	Disable()                // Переводит flow control в невалидное состояние
	Add(n uint32)            // Увеличение размера окна на отправку
	Reset(n uint32)          // Сбрасывает состояние
}
