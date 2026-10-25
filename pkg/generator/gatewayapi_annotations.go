package generator

type annotationResult struct {
	RuleFilters      []map[string]interface{} // appended to every rule that has backendRefs
	PathRewrite      func(p IngressPath) (match map[string]interface{}, filters []map[string]interface{}, ok bool)
	ExtraRules       []map[string]interface{} // app-root, redirects (first in the route)
	ForceRegex       bool                     // use-regex
	GRPC             bool                     // backend-protocol GRPC/GRPCS
	SSLRedirect      *bool                    // nil = controller default
	RouteAnnotations map[string]string        // implementation=alb
	Extended         []string                 // Extended features used, for the Note
}
