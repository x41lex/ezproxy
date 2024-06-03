package api

import (
	"encoding/json"
	"net/http"
)

func writeResponse(w http.ResponseWriter, status int, data interface{}) error {
	jdata, err := json.Marshal(baseResponse{
		Status: status,
		Data:   data,
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return err
	}
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(200)
	_, err = w.Write(jdata)
	return err
}
