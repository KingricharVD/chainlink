package bulletprooftxmanager

func XXXTriggerChan(eb EthBroadcaster) <-chan struct{} {
	return eb.(*ethBroadcaster).trigger
}

func XXXMustStartEthTxInsertListener(eb EthBroadcaster) {
	if err := eb.(*ethBroadcaster).ethTxInsertListener.Start(); err != nil {
		panic(err)
	}
	go eb.(*ethBroadcaster).ethTxInsertTriggerer()
}
