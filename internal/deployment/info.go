// Package deployment describes network access without inspecting Docker or changing the host.
package deployment

import (
	"fmt"
	"netlite/internal/model"
	"strconv"
	"strings"
)

type Mapping struct {
	PublicPort int `json:"public_port"`
	ListenPort int `json:"listen_port"`
}

type Info struct {
	Mode          string    `json:"mode"`
	MinPort       int       `json:"min_port"`
	Mappings      []Mapping `json:"mappings"`
	MappingsKnown bool      `json:"mappings_known"`
}

func Parse(mode, mappings string, gateway bool) (Info, error) {
	if gateway {
		mode = "fnos"
	}
	if mode == "" {
		mode = "native"
	}
	i := Info{Mode: mode, MinPort: 1, Mappings: []Mapping{}}
	switch mode {
	case "fnos":
		i.MinPort = 1024
	case "native", "host", "bridge", "unknown":
	default:
		return i, fmt.Errorf("invalid NETLITE_NETWORK_MODE: %q", mode)
	}
	i.MappingsKnown = mode == "bridge" && mappings != ""
	for _, entry := range strings.Split(mappings, ",") {
		if entry == "" {
			continue
		}
		parts := strings.Split(entry, ":")
		if len(parts) != 2 {
			return i, fmt.Errorf("NETLITE_PORT_MAPPINGS must be public:listen pairs")
		}
		public, e1 := strconv.Atoi(parts[0])
		listen, e2 := strconv.Atoi(parts[1])
		if e1 != nil || e2 != nil || public < 1 || public > 65535 || listen < 1 || listen > 65535 {
			return i, fmt.Errorf("invalid NETLITE_PORT_MAPPINGS entry %q", entry)
		}
		for _, old := range i.Mappings {
			if old.PublicPort == public {
				return i, fmt.Errorf("duplicate public port %d", public)
			}
		}
		i.Mappings = append(i.Mappings, Mapping{public, listen})
	}
	return i, nil
}

func (i Info) Validate(cfg model.Config) error {
	for _, r := range cfg.Routes {
		if r.Port < i.MinPort {
			return fmt.Errorf("当前部署的代理端口须为 %d..65535；fnOS 不接管系统 Web 服务", i.MinPort)
		}
	}
	return nil
}
