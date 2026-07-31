package features

import "sync"

// Pool manages feature clients keyed by project for the same server URL.
// Concurrent calls for the same project reuse the same Client and its cache.
type Pool struct {
	serverURL string
	opts      []ConfigureOption

	mu      sync.Mutex
	clients map[string]*Client
}

// NewPool creates a pool that lazily builds clients for each project.
func NewPool(serverURL string, opts ...ConfigureOption) *Pool {
	return &Pool{
		serverURL: serverURL,
		opts:      opts,
		clients:   make(map[string]*Client),
	}
}

// Client returns the cached client for project, creating it on first use.
func (p *Pool) Client(project string) *Client {
	p.mu.Lock()
	defer p.mu.Unlock()

	if c, ok := p.clients[project]; ok {
		return c
	}

	c := newClient(p.serverURL, project, configureOptionsFrom(p.opts))
	p.clients[project] = c
	return c
}

// Flag evaluates a flag for the given project.
func (p *Pool) Flag(project, code string, opts ...FlagOption) bool {
	return p.Client(project).Flag(code, opts...)
}

// Close closes all clients in the pool.
func (p *Pool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for project, c := range p.clients {
		c.Close()
		delete(p.clients, project)
	}
}
