package kotomiMsg

import (
  "github.com/fiathux/wsdc-wdk-go/rfc/uuid"
)

// ------------------------ INTERFACE ------------------------{{{

type MsgTracker_i interface {
  EventConnect(svr,conn uuid.UUID_t)
  EventDown(svr,conn uuid.UUID_t)
  DataRecv(svr,conn uuid.UUID_t)
  Distory()
}

// ------------------------ INTERFACE END ------------------------}}}

