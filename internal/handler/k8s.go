package handler

import (
	"fmt"
	"net/http"
)

type K8sHandler struct{}

func NewK8sHandler() *K8sHandler {
	return &K8sHandler{}
}

func (h *K8sHandler) Health(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "OK")
}

func (h *K8sHandler) Ready(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "READY")
}
