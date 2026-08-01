package agent

import "github.com/fabith10/synapse-go/internal/agent/utils"

type CustomWarningCheck = utils.CustomWarningCheck
type SystemWarningsConfig = utils.SystemWarningsConfig

func LoadSystemWarningsConfig(path string) error {
	return utils.LoadSystemWarningsConfig(path)
}

func SetSystemWarningsConfig(cfg SystemWarningsConfig) {
	utils.SetSystemWarningsConfig(cfg)
}

func GetSystemWarningsConfig() SystemWarningsConfig {
	return utils.GetSystemWarningsConfig()
}

func CheckSystemWarnings() []string {
	return utils.CheckSystemWarnings(func() (map[string]bool, func(string) string) {
		activeTools := make(map[string]bool)
		configs := GetLoadedAgentConfigs()
		for _, agentCfg := range configs {
			for _, t := range agentCfg.Tools {
				activeTools[ResolveToolAlias(t)] = true
			}
		}
		return activeTools, ResolveToolAlias
	})
}
