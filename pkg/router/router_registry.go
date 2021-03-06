package router

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/bitly/go-simplejson"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/grpc-ecosystem/grpc-gateway/v2/utilities"
	"github.com/hb-go/grpc-contrib/client"
	"github.com/hb-go/grpc-contrib/log"
	"github.com/hb-go/grpc-contrib/registry"
	"github.com/hb-go/grpc-contrib/registry/cache"
	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"

	"github.com/hb-chen/gateway/v2/pkg/codec"
	"github.com/hb-chen/gateway/v2/pkg/proto"
)

var valuesKeyRegexp = regexp.MustCompile(`^(.*)\[(.*)\]$`)

type Route struct {
	method         string
	pattern        runtime.Pattern
	serviceName    string
	methodName     string
	serviceVersion []string
}

// router is the default router
type registryRouter struct {
	exit chan bool
	opts Options

	// registry cache
	rc cache.Cache

	sync.RWMutex
	eps    map[string]*registry.Service
	routes map[string]*Route
}

func (r *registryRouter) isClosed() bool {
	select {
	case <-r.exit:
		return true
	default:
		return false
	}
}

// refresh list of api services
func (r *registryRouter) refresh() {
	var attempts int

	for {
		services, err := r.opts.registry.ListServices()
		if err != nil {
			attempts++
			grpclog.Errorf("unable to list services: %v", err)
			time.Sleep(time.Duration(attempts) * time.Second)
			continue
		}

		attempts = 0

		// for each service, get service and store endpoints
		for _, s := range services {
			// only get services for this namespace
			if !strings.HasPrefix(s.Name, r.opts.namespace) {
				continue
			}
			service, err := r.rc.GetService(s.Name)
			if err != nil {
				grpclog.Error("unable to get service: %v error: %v", s.Name, err)
				continue
			}
			r.store(service)
		}

		// refresh list in 10 minutes... cruft
		select {
		case <-time.After(time.Minute * 10):
		case <-r.exit:
			return
		}
	}
}

// process watch event
func (r *registryRouter) process(res *registry.Result) {
	// skip these things
	if res == nil || res.Service == nil {
		return
	}

	grpclog.Infof("process action: %v, service: %v", res.Action, log.StringJSON(res.Service))

	// namespace filter
	if !strings.HasPrefix(res.Service.Name, r.opts.namespace) {
		return
	}

	// skip these things
	if res == nil || res.Service == nil || !strings.HasPrefix(res.Service.Name, r.opts.namespace) {
		return
	}

	// TODO ????????????????????????????????????????????????????????????????????????????????????????????? refresh ????????????????????????

	// get entry from cache
	service, err := r.rc.GetService(res.Service.Name)
	if res.Action == "delete" && err == registry.ErrNotFound {
		// delete ??? cache ???????????? service??????????????????
		// ??????????????????
		service = []*registry.Service{
			{
				Name:     res.Service.Name,
				Version:  res.Service.Version,
				Metadata: nil,
				Methods:  nil,
				Nodes:    nil,
			},
		}
		r.store(service)
		return
	} else if err != nil {
		grpclog.Errorf("unable to get service:%v error: %v", res.Service.Name, err)
		return
	}

	// update our local endpoints
	r.store(service)
}

// store local endpoint cache
func (r *registryRouter) store(services []*registry.Service) {
	grpclog.Infof("store services: %v", log.StringJSON(services))

	// endpoints
	eps := map[string]*registry.Service{}
	routes := map[string]*Route{}
	// create a new endpoint mapping
	for _, service := range services {
		// ?????????????????????????????????????????????+????????????????????????
		for _, m := range service.Methods {
			for _, binding := range m.Bindings {
				pattern := runtime.MustPattern(runtime.NewPattern(
					binding.PathTmpl.Version,
					binding.PathTmpl.OpCodes,
					binding.PathTmpl.Pool,
					binding.PathTmpl.Verb,
				))

				k := fmt.Sprintf("%s:%s", binding.Method, pattern.String())
				if r, ok := routes[k]; ok {
					if r.serviceName != service.Name || r.methodName != m.Name {
						grpclog.Warningf("route have different service or method")
						continue
					}
					r.serviceVersion = append(r.serviceVersion, service.Version)
				} else {
					r := &Route{
						method:         binding.Method,
						pattern:        pattern,
						serviceName:    service.Name,
						methodName:     m.Name,
						serviceVersion: []string{service.Version},
					}

					routes[k] = r
				}

			}
		}

		// create a key service:endpoint_name
		key := fmt.Sprintf("%s:%s", service.Name, service.Version)
		eps[key] = service
	}

	r.Lock()
	defer r.Unlock()

	// ?????????
	for key, route := range routes {
		r.opts.mux.Handle(
			route.method,
			route.pattern,
			r.handler(route.serviceName, route.methodName, route.serviceVersion),
		)

		r.routes[key] = route
	}

	// ?????????
	for key, service := range eps {
		// ????????????????????????????????????
		if svc, ok := r.eps[key]; ok {
			for _, m := range svc.Methods {
				for _, binding := range m.Bindings {
					pattern := runtime.MustPattern(runtime.NewPattern(
						binding.PathTmpl.Version,
						binding.PathTmpl.OpCodes,
						binding.PathTmpl.Pool,
						binding.PathTmpl.Verb,
					))

					k := fmt.Sprintf("%s:%s", binding.Method, pattern.String())
					// ?????????????????????????????????????????????
					if _, ok := routes[k]; !ok {
						// ?????????????????????????????? service ?????????
						if ro, ok := r.routes[k]; ok && ro.serviceName != svc.Name {
							continue
						}

						r.opts.mux.HandlerDeregister(binding.Method, pattern)
					}
				}
			}
		}

		r.eps[key] = service
	}
}

// watch for endpoint changes
func (r *registryRouter) watch() {
	var attempts int

	for {
		if r.isClosed() {
			return
		}

		// watch for changes
		w, err := r.opts.registry.Watch()
		if err != nil {
			attempts++
			grpclog.Errorf("error watching endpoints: %v", err)
			time.Sleep(time.Duration(attempts) * time.Second)
			continue
		}

		ch := make(chan bool)

		go func() {
			select {
			case <-ch:
				w.Stop()
			case <-r.exit:
				w.Stop()
			}
		}()

		// reset if we get here
		attempts = 0

		for {
			// process next event
			res, err := w.Next()
			if err != nil {
				grpclog.Errorf("error getting next endoint: %v", err)
				close(ch)
				break
			}
			r.process(res)
		}
	}
}

// TODO ???????????????
func (r *registryRouter) handler(serviceName, method string, versions []string) runtime.HandlerFunc {
	marshaler := &runtime.JSONPb{}
	return func(w http.ResponseWriter, req *http.Request, pathParams map[string]string) {
		grpclog.Infof("handler service name: %v, method: %v, versions: %v", serviceName, method, versions)

		payload := &simplejson.Json{}
		switch req.Method {
		case "PATCH", "POST", "PUT", "DELETE":
			// Body to JSON
			newReader, err := utilities.IOReaderFactory(req.Body)
			if err != nil {
				runtime.HTTPError(context.TODO(), r.opts.mux.ServeMux, marshaler, w, req, err)
				return
			}
			payload, err = simplejson.NewFromReader(newReader())
			if err != nil {
				runtime.HTTPError(context.TODO(), r.opts.mux.ServeMux, marshaler, w, req, err)
				return
			}
		}

		// Path params
		for key, val := range pathParams {
			fieldPath := strings.Split(key, ".")
			payload.SetPath(fieldPath, val)
		}

		// Query params
		// ?????????????????????
		// ?????????????????? grpc-gateway ???????????? path ???????????????????????? query???????????????????????????????????? request ??????
		if err := req.ParseForm(); err != nil {
			runtime.HTTPError(context.TODO(), r.opts.mux.ServeMux, marshaler, w, req, err)
			return
		}
		for key, values := range req.Form {
			match := valuesKeyRegexp.FindStringSubmatch(key)
			if len(match) == 3 {
				key = match[1]
				values = append([]string{match[2]}, values...)
			}
			fieldPath := strings.Split(key, ".")

			if payload.GetPath(fieldPath...).Interface() == nil {
				payload.SetPath(fieldPath, values)
			}
		}

		data, err := payload.MarshalJSON()
		if err != nil {
			runtime.HTTPError(context.TODO(), r.opts.mux.ServeMux, marshaler, w, req, err)
			return
		}

		resp := &proto.Message{}
		rpcReq := proto.NewMessage(data)
		grpclog.Infof("req: %+v", rpcReq)

		s := registry.Service{Name: serviceName}
		cc, closer, err := client.Client(
			&s,
			client.WithRegistryOptions(registry.Versions(versions...)),
		)
		if err != nil {
			runtime.HTTPError(context.TODO(), r.opts.mux.ServeMux, marshaler, w, req, err)
			return
		}
		defer closer.Close()

		ctx, cancel := context.WithCancel(req.Context())
		defer cancel()
		rctx, err := runtime.AnnotateContext(ctx, r.opts.mux.ServeMux, req, "")
		if err != nil {
			runtime.HTTPError(ctx, r.opts.mux.ServeMux, marshaler, w, req, err)
			return
		}

		var metadata runtime.ServerMetadata
		method := fmt.Sprintf("/%s/%s", serviceName, method)
		err = cc.Invoke(rctx, method, rpcReq, resp,
			grpc.CallContentSubtype(codec.CODEC_JSON),
			grpc.Header(&metadata.HeaderMD),
			grpc.Trailer(&metadata.TrailerMD),
		)
		if err != nil {
			runtime.HTTPError(ctx, r.opts.mux.ServeMux, marshaler, w, req, err)
			return
		}

		ctx = runtime.NewServerMetadataContext(ctx, metadata)
		if err != nil {
			runtime.HTTPError(ctx, r.opts.mux.ServeMux, marshaler, w, req, err)
			return
		}

		runtime.ForwardResponseMessage(ctx, r.opts.mux.ServeMux, resp, w, req, nil)
	}
}

func (r *registryRouter) Options() Options {
	return r.opts
}

func (r *registryRouter) Close() error {
	select {
	case <-r.exit:
		return nil
	default:
		close(r.exit)
		r.rc.Stop()
	}
	return nil
}

func newRouter(opts ...Option) *registryRouter {
	options := NewOptions(opts...)
	r := &registryRouter{
		exit:   make(chan bool),
		opts:   options,
		rc:     cache.New(options.registry),
		eps:    make(map[string]*registry.Service),
		routes: make(map[string]*Route),
	}
	go r.watch()
	go r.refresh()
	return r
}

// NewRouter returns the default router
func NewRouter(opts ...Option) *registryRouter {
	r := newRouter(opts...)
	registry.RegisterBuilder(r.opts.registry)
	return r
}
