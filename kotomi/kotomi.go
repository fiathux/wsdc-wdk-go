package kotomi

import (
  "github.com/fiathux/wsdc-wdk-go/rfc/uuid"
  "github.com/fiathux/wsdc-wdk-go/kotomi/iohub"
  "sync"
  "encoding/binary"
  _"regexp"
)

const PROTOCOL_MODULE_FRAMELEN = 65536

// ------------------------ TYPE DEFINE ------------------------{{{

type trackFeature int   //Identification feature
const (
  FT_TK_SUB trackFeature = iota //Subscription group
  FT_TK_HASH                    //Hash group
  FT_TK_MOD                     //Module group
  FT_TK_CLIENT                  //Client group
)

type track_t struct {
  id uuid.UUID_t
  bindA iohub.Service_i
  bindB iohub.Service_i
  bindFuncA uuid.UUID_t
  bindFuncB uuid.UUID_t
  rdFilterA trackFilter
  rdFilterB trackFilter
  name string
}

type TrackInfo struct {
  Id uuid.UUID_t
  Name string
  ChanA uuid.UUID_t
  ChanB uuid.UUID_t
}

type ServiceInfo struct {
  Id uuid.UUID_t
  Addr string
}

type trackFilter func(frm *iohub.Frame_t) (out []byte, dist []uuid.UUID_t)
 
// ------------------------ TYPE DEFINE END ------------------------}}}

// ------------------------ INTERFACE ------------------------{{{

// ------------------------ INTERFACE END ------------------------}}}

// ------------------------ MODULE PRIVATE METHOD ------------------------{{{

var tabService map[uuid.UUID_t] iohub.Service_i
var tabTrack map[uuid.UUID_t] track_t
var tabCallback map[uuid.UUID_t] iohub.Service_i        //Track or Callable mapping to Service
var tabServiceRef map[uuid.UUID_t] int
var moduleSync sync.RWMutex

//Initialization
func init(){
  tabService=make(map[uuid.UUID_t] iohub.Service_i)
  tabTrack=make(map[uuid.UUID_t] track_t)
  tabCallback=make(map[uuid.UUID_t] iohub.Service_i)
  tabServiceRef=make(map[uuid.UUID_t] int)
  moduleSync=sync.RWMutex{}
}

//Module read-lock decorator
func readDecorator(proc func()) {
  moduleSync.RLock()
  defer moduleSync.RUnlock()
  proc()
}

//Module lock decorator
func uniqueDecorator(proc func()) {
  moduleSync.Lock()
  defer moduleSync.Unlock()
  proc()
}

//Add service reference
func serviceRefAdd(serv uuid.UUID_t) {
  _,ok := tabServiceRef[serv]
  if ok {
    tabServiceRef[serv] += 1
  }else{
    tabServiceRef[serv] = 1
  }
}

//Remove service reference
func serviceRefDec(serv uuid.UUID_t) {
  _,ok := tabServiceRef[serv]
  if ok {
    tabServiceRef[serv] -= 1
    if tabServiceRef[serv] == 0 {
      delete(tabServiceRef,serv)
    }
  }
}

type mPotoFrame_t struct {
  id uuid.UUID_t
  data []byte
  command string
  tag mPotoTag_t
}

//Modules protocol process {{{
type mPotoIter_t func(data []byte)(frmstack []mPotoFrame_t,nextparse mPotoIter_t)
type mPotoTag_t uint
type MPotoTag_t mPotoTag_t
const (
  MPROTO_DATA mPotoTag_t = iota
  MPROTO_CONNECT
  MPROTO_DISCONNECT
  MPROTO_LAST         //Write last frame
)

const MPROTO_STACK_CAP = 4
const MPROTO_HEAD_LEN int = 24
const MPROTO_BODY_LEN int = PROTOCOL_MODULE_FRAMELEN - MPROTO_HEAD_LEN
const MPROTO_MAGIC string = "TSP:"
const MPROTO_CMD_SPLIT string = "\r\n"

func moduleProtocolBuild() []mPotoFrame_t {
}

//Create module protocol parser{{{
func moduleProtocolEntry() mPotoIter_t {
  frmbuff := make([]byte,0,PROTOCOL_MODULE_FRAMELEN)
  bodylen := 0
  pf := mPotoFrame_t{}
  var headParser mPotoIter_t
  var bodyParser mPotoIter_t
  //Head
  headParser = func(data []byte) ([]mPotoFrame_t,mPotoIter_t) {
    frmbuff = append(frmbuff,data...)
    if len(frmbuff) < MPROTO_HEAD_LEN {
      return nil,headParser
    }else{
      if string(frmbuff[0:4]) != MPROTO_MAGIC {
        return nil,nil
      }
      copy(pf.id[:],frmbuff[4:20])
      inflen := binary.BigEndian.Uint32(frmbuff[20:24])
      pf.tag = mPotoTag_t(inflen>>28)
      bodylen = int(inflen & 0x0fffffff)
      if bodylen > MPROTO_BODY_LEN{
        return nil,nil
      }
      return bodyParser(frmbuff[24:])
    }
  }
  //Body
  bodyParser = func(data []byte) ([]mPotoFrame_t,mPotoIter_t){
    if len(data) < bodylen { return nil,bodyParser }
    bdata := data[0:bodylen]
    data = data[bodylen:]
    stackFrm,flowPars := moduleProtocolEntry()(data)
    pf.data = func () []byte{
      for i:=2; i<len(bdata); i+=2 {
        if string(bdata[i-2:i]) == MPROTO_CMD_SPLIT {
          pf.command = string(bdata[0:i-2])
          return bdata[i:]
        }
      }
      return make([]byte,0)
    }()
    if stackFrm != nil {
      return append(stackFrm,pf),flowPars
    }else{
      fmstack := make([]mPotoFrame_t,0,MPROTO_STACK_CAP);
      return append(fmstack,pf),flowPars
    }
  }
  return headParser
}
//}}}

//}}}

// ------------------------ MODULE PRIVATE METHOD END ------------------------}}}

// ------------------------ MODULE PUBLIC METHOD ------------------------{{{

//Status check{{{

//
func EnumTracks() []TrackInfo {
  trkchan := make(chan []TrackInfo)
  go readDecorator(func(){
    if len(tabTrack) == 0 {
      trkchan <- nil
      return
    }
    trk:=make([]TrackInfo,0,len(tabTrack))
    for _,v:= range tabTrack {
      trk = append(trk, TrackInfo{v.id, v.name, v.bindA.GetID(), v.bindB.GetID()})
    }
    trkchan <- trk
  })
  return <-trkchan
}

//
func GetTrackInfo(trackid uuid.UUID_t) *TrackInfo {
  trkchan := make(chan *TrackInfo)
  go readDecorator(func(){
    trk,ok := tabTrack[trackid]
    if ok {
      trkinf := TrackInfo{trk.id, trk.name, trk.bindA.GetID(), trk.bindB.GetID()}
      trkchan <- &trkinf
    }else{
      trkchan <- nil
    }
  })
  return <-trkchan
}

//
func EnumServices() []ServiceInfo {
  svrchan := make(chan []ServiceInfo)
  go readDecorator(func(){
    if len(tabService) == 0 {
      svrchan <- nil
      return
    }
    svr:=make([]ServiceInfo,0,len(tabService))
    for _,v:= range tabService {
      svr = append(svr, ServiceInfo{v.GetID(), v.GetAddr()})
    }
    svrchan <- svr
  })
  return <-svrchan
}

//
func GetServiceInfo(svrid uuid.UUID_t) *ServiceInfo {
  svrchan := make(chan *ServiceInfo)
  go readDecorator(func(){
    svr,ok := tabService[svrid]
    if ok {
      svrinf := ServiceInfo{svr.GetID(), svr.GetAddr()}
      svrchan <- &svrinf
    }else{
      svrchan <- nil
    }
  })
  return <-svrchan
}

//}}}

//
func BeginService(protocol string,addr string) uuid.UUID_t {
  var nettype iohub.NetType
  switch protocol {
  case "tcp":
    nettype = iohub.NT_TCP
  case "udp":
    nettype = iohub.NT_UDP
  case "unix":
    nettype = iohub.NT_UNIX
  case "unixpacket":
    nettype = iohub.NT_UNIXPACK
  default:
    return uuid.UUIDNull()
  }
  svr := iohub.CreateService(nettype,addr)
  if svr == nil {
    return uuid.UUIDNull()
  }
  uniqueDecorator(func(){
    tabService[svr.GetID()] = svr
  })
  return svr.GetID()
}

//
func QuickBindTrack(rule string,name string,servA uuid.UUID_t,servB uuid.UUID_t) uuid.UUID_t {
  switch {
  case rule == "T":   //Transparent broadcast
    return BindTrack(name,nil,nil,servA,servB)
  case rule == "C":   //Client to modlue-format
    //
  case rule == "M":   //Module-format broadcast
    //
  case rule == "P":   //Point to point format
    //
  case rule[0:2] == "M*":  //Subscript module-format broadcast (buffered)
    //
  case rule[0:2] == "M#":  //Module-format hash distrabute (buffered)
    //
  }
  return uuid.UUIDNull()
}

//
func BindTrack(name string, filterA trackFilter, filterB trackFilter,
servA uuid.UUID_t,servB uuid.UUID_t) uuid.UUID_t {
  //Make service reader with filter{{{
  makeReader := func(distsvr iohub.Service_i) func(trackFilter) func(*iohub.Frame_t) {
    return func (flt trackFilter) func(*iohub.Frame_t) {
      if flt == nil { //No filter
        return func (frm *iohub.Frame_t) {
          //Todo: output message log
          if len(frm.Data)>0 {
            distsvr.Write(iohub.Frame_t{ID:uuid.UUIDNull(),Data:frm.Data,Err:frm.Err,
            Event:iohub.SEVT_BCAST})
          }
        }
      }else{
        return func (frm *iohub.Frame_t) {
          fdata,fdist := flt(frm)
          if fdata != nil && len(fdata)>0 && fdist != nil && len(fdist)>0 {
            for _,i := range fdist {
              distsvr.Write(iohub.Frame_t{ID:i, Data:fdata, Event:iohub.SEVT_BCAST})
            }
          }
        }
      }
    }
  }
  //}}}
  idchan := make(chan uuid.UUID_t)
  go uniqueDecorator(func(){
    svrAObj,ok := tabService[servA]
    if !ok {
      idchan <- uuid.UUIDNull()
      return
    }
    svrBObj,ok := tabService[servB]
    if !ok {
      idchan <- uuid.UUIDNull()
      return
    }
    trk := track_t{ id:uuid.UUID1(), bindA:svrAObj, bindB:svrBObj, name:name,
    rdFilterA:filterA, rdFilterB:filterB, bindFuncA:uuid.UUID1(), bindFuncB:uuid.UUID1() }
    svrAObj.RegReader(trk.bindFuncA,makeReader(svrBObj)(trk.rdFilterA))
    svrBObj.RegReader(trk.bindFuncB,makeReader(svrAObj)(trk.rdFilterB))
    tabTrack[trk.id] = trk
    serviceRefAdd(servA)
    serviceRefAdd(servB)
    idchan <- trk.id
  })
  return <-idchan
}

//
func BindCallback(proc func(frm *iohub.Frame_t),serv uuid.UUID_t) uuid.UUID_t {
  idchan := make(chan uuid.UUID_t)
  go uniqueDecorator(func(){
    callid := uuid.UUID1()
    svrObj,ok := tabService[serv]
    if !ok {
      idchan <- uuid.UUIDNull()
      return
    }
    svrObj.RegReader(callid,proc)
    tabCallback[callid] = svrObj
    serviceRefAdd(serv)
  })
  return <-idchan
}

//
func RemoveTrack(id uuid.UUID_t) {
  go uniqueDecorator(func(){
    trk,ok := tabTrack[id]
    if !ok {return}
    trk.bindA.UnregReader(trk.bindFuncA)
    serviceRefDec(trk.bindA.GetID())
    trk.bindB.UnregReader(trk.bindFuncB)
    serviceRefDec(trk.bindB.GetID())
    delete(tabTrack,id)
  })
}

//
func RemoveCall(id uuid.UUID_t) {
  go uniqueDecorator(func(){
    csvr,ok := tabCallback[id]
    if !ok {return}
    csvr.UnregReader(id)
    serviceRefDec(csvr.GetID())
    delete(tabCallback,id)
  })
}

//
func CloseService(id uuid.UUID_t) bool {
  oprst := make(chan bool)
  go uniqueDecorator(func(){
    _,have := tabService[id]
    if have {
      _,noref := tabServiceRef[id]
      if !noref {
        oprst <- true
        return
      }
    }
    oprst <- false
  })
  return <- oprst
}

//
func WriteTo(service uuid.UUID_t,session uuid.UUID_t,data []byte) bool {
  oprst := make(chan bool)
  go readDecorator(func(){
    svr,ok := tabService[service]
    if !ok {
      oprst <- false
      return
    }
    var evt iohub.SessionEvent
    if session.IsNull() { evt=iohub.SEVT_BCAST }else{ evt=iohub.SEVT_DATA }
    oprst <- svr.Write(iohub.Frame_t{ID:session, Data:data, Event:evt})
  })
  return <- oprst
}

// ------------------------ MODULE PUBLIC METHOD END ------------------------}}}

