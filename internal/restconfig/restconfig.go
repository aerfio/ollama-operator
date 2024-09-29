package restconfig

import "k8s.io/client-go/rest"

func Adjust(restCfg *rest.Config, qps float32, burst int, userAgent string) {
	restCfg.QPS = qps
	restCfg.Burst = burst
	restCfg.UserAgent = userAgent
}
