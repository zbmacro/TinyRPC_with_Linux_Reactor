package reactor

// worker池，处理业务逻辑
func createWorker(workTask chan *WorkerTask, server Server, handlerWriteTask chan *WorkerTask) {
	for work := range workTask {
		server.HandleRequest(work, handlerWriteTask)
	}
}
