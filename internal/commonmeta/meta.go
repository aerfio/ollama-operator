package commonmeta

import "maps"

var (
	ManagedByLabel  = map[string]string{"app.kubernetes.io/managed-by": "ollama-operator"}
	AppNameLabelKey = "app.kubernetes.io/name"
)

func LabelsForResource(resourceName string, other ...map[string]string) map[string]string {
	m := map[string]string{
		AppNameLabelKey: resourceName,
	}
	maps.Copy(m, ManagedByLabel)
	for _, m2 := range other {
		maps.Copy(m, m2)
	}

	return m
}
