package kong

// Kong Admin API entities, limited to the fields the migration uses.

type Ref struct {
	ID string `json:"id"`
}

type Service struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Protocol          string   `json:"protocol"`
	Host              string   `json:"host"`
	Port              int      `json:"port"`
	Path              string   `json:"path"`
	Retries           *int     `json:"retries"`
	ConnectTimeout    int      `json:"connect_timeout"`
	WriteTimeout      int      `json:"write_timeout"`
	ReadTimeout       int      `json:"read_timeout"`
	TLSVerify         *bool    `json:"tls_verify"`
	ClientCertificate *Ref     `json:"client_certificate"`
	Enabled           *bool    `json:"enabled"`
	Tags              []string `json:"tags"`
}

type Route struct {
	ID                      string              `json:"id"`
	Name                    string              `json:"name"`
	Protocols               []string            `json:"protocols"`
	Methods                 []string            `json:"methods"`
	Hosts                   []string            `json:"hosts"`
	Paths                   []string            `json:"paths"`
	Headers                 map[string][]string `json:"headers"`
	SNIs                    []string            `json:"snis"`
	Sources                 []any               `json:"sources"`
	Destinations            []any               `json:"destinations"`
	Expression              string              `json:"expression"`
	StripPath               bool                `json:"strip_path"`
	PreserveHost            bool                `json:"preserve_host"`
	RegexPriority           int                 `json:"regex_priority"`
	PathHandling            string              `json:"path_handling"`
	HTTPSRedirectStatusCode int                 `json:"https_redirect_status_code"`
	RequestBuffering        *bool               `json:"request_buffering"`
	ResponseBuffering       *bool               `json:"response_buffering"`
	Service                 *Ref                `json:"service"`
	Tags                    []string            `json:"tags"`
}

type Upstream struct {
	ID                string         `json:"id"`
	Name              string         `json:"name"`
	Algorithm         string         `json:"algorithm"`
	HashOn            string         `json:"hash_on"`
	HashFallback      string         `json:"hash_fallback"`
	HashOnHeader      string         `json:"hash_on_header"`
	HashOnCookie      string         `json:"hash_on_cookie"`
	HashOnQueryArg    string         `json:"hash_on_query_arg"`
	HashOnURICapture  string         `json:"hash_on_uri_capture"`
	HostHeader        string         `json:"host_header"`
	Healthchecks      map[string]any `json:"healthchecks"`
	ClientCertificate *Ref           `json:"client_certificate"`
	Tags              []string       `json:"tags"`
}

type Target struct {
	ID       string `json:"id"`
	Target   string `json:"target"`
	Weight   int    `json:"weight"`
	Upstream Ref    `json:"upstream"`
}

type Plugin struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Enabled       bool           `json:"enabled"`
	Config        map[string]any `json:"config"`
	Service       *Ref           `json:"service"`
	Route         *Ref           `json:"route"`
	Consumer      *Ref           `json:"consumer"`
	ConsumerGroup *Ref           `json:"consumer_group"`
	Protocols     []string       `json:"protocols"`
	Tags          []string       `json:"tags"`
}

type Consumer struct {
	ID       string   `json:"id"`
	Username string   `json:"username"`
	CustomID string   `json:"custom_id"`
	Tags     []string `json:"tags"`
}

type KeyAuth struct {
	ID       string `json:"id"`
	Key      string `json:"key"`
	TTL      int    `json:"ttl"`
	Consumer Ref    `json:"consumer"`
}

type BasicAuth struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Consumer Ref    `json:"consumer"`
}

type JWT struct {
	ID           string `json:"id"`
	Key          string `json:"key"`
	Secret       string `json:"secret"`
	Algorithm    string `json:"algorithm"`
	RSAPublicKey string `json:"rsa_public_key"`
	Consumer     Ref    `json:"consumer"`
}

type HMACAuth struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Secret   string `json:"secret"`
	Consumer Ref    `json:"consumer"`
}

type ACL struct {
	ID       string `json:"id"`
	Group    string `json:"group"`
	Consumer Ref    `json:"consumer"`
}

// Snapshot is the whole Kong configuration relevant to the migration.
type Snapshot struct {
	Services   []Service           `json:"services"`
	Routes     []Route             `json:"routes"`
	Upstreams  []Upstream          `json:"upstreams"`
	Targets    map[string][]Target `json:"targets"` // by upstream id
	Plugins    []Plugin            `json:"plugins"`
	Consumers  []Consumer          `json:"consumers"`
	KeyAuths   []KeyAuth           `json:"keyauth_credentials"`
	BasicAuths []BasicAuth         `json:"basicauth_credentials"`
	JWTs       []JWT               `json:"jwt_secrets"`
	HMACAuths  []HMACAuth          `json:"hmacauth_credentials"`
	ACLs       []ACL               `json:"acls"`

	services       map[string]*Service
	upstreamByName map[string]*Upstream
	consumers      map[string]*Consumer
	consumerByName map[string]*Consumer
	routesBySvc    map[string][]*Route
	groups         map[string][]string // consumer id → ACL groups
}

func (s *Snapshot) index() {
	s.services = map[string]*Service{}
	for i := range s.Services {
		s.services[s.Services[i].ID] = &s.Services[i]
	}
	s.upstreamByName = map[string]*Upstream{}
	for i := range s.Upstreams {
		s.upstreamByName[s.Upstreams[i].Name] = &s.Upstreams[i]
	}
	s.consumers = map[string]*Consumer{}
	s.consumerByName = map[string]*Consumer{}
	for i := range s.Consumers {
		c := &s.Consumers[i]
		s.consumers[c.ID] = c
		if c.Username != "" {
			s.consumerByName[c.Username] = c
		}
	}
	s.routesBySvc = map[string][]*Route{}
	for i := range s.Routes {
		r := &s.Routes[i]
		svc := ""
		if r.Service != nil {
			svc = r.Service.ID
		}
		s.routesBySvc[svc] = append(s.routesBySvc[svc], r)
	}
	s.groups = map[string][]string{}
	for _, a := range s.ACLs {
		s.groups[a.Consumer.ID] = append(s.groups[a.Consumer.ID], a.Group)
	}
	if s.Targets == nil {
		s.Targets = map[string][]Target{}
	}
}

// consumer resolves a reference made by id or username (e.g. "anonymous").
func (s *Snapshot) consumer(ref string) *Consumer {
	if c := s.consumers[ref]; c != nil {
		return c
	}
	return s.consumerByName[ref]
}
