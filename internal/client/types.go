package client

type InvocationRequest struct {
	Params          map[string]interface{}
	QoSClass        int64
	QoSMaxRespT     float64
	CanDoOffloading bool
	Async           bool
	ReturnOutput    bool
}

type PrewarmingRequest struct {
	Function       string
	CPUDemand	   float64
	Instances      int64
	ForceImagePull bool
}
