package kotomiMsg

import (
  "github.com/fiathux/wsdc-wdk-go/rfc/uuid"
  "github.com/fiathux/wsdc-wdk-go/kotomiMsg/iohub"
  "sync"
  "time"
  _"regexp"
)

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
  rdBinderA TrackBinder
  rdBinderB TrackBinder
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

type CommandInfo struct {
  Id uuid.UUID_t
  Cmd string
}

type TrackFilter func(frm *iohub.Frame_t) (out []*iohub.Frame_t,answ *iohub.Frame_t)
type TrackDistributer func(outfrm []*iohub.Frame_t) []*iohub.Frame_t
type TrackBinder func(distsvr,srcsvr iohub.Service_i) func(frm *iohub.Frame_t)
type TrackCmdProc func(from uuid.UUID_t,command string)
 
// ------------------------ TYPE DEFINE END ------------------------}}}

// ------------------------ MODULE PRIVATE METHOD ------------------------{{{

var tabService map[uuid.UUID_t] iohub.Service_i
var tabTrack map[uuid.UUID_t] track_t
var tabCallback map[uuid.UUID_t] iohub.Service_i        //Track or Callable mapping to Service
var tabServiceRef map[uuid.UUID_t] int
var tabCmdReader map[uuid.UUID_t] TrackCmdProc
var cmdChannel chan CommandInfo
var moduleSync sync.RWMutex
var cmdProcSync sync.RWMutex

//Initialization
func init(){
  tabService=make(map[uuid.UUID_t] iohub.Service_i)
  tabTrack=make(map[uuid.UUID_t] track_t)
  tabCallback=make(map[uuid.UUID_t] iohub.Service_i)
  tabServiceRef=make(map[uuid.UUID_t] int)
  tabCmdReader=make(map[uuid.UUID_t] TrackCmdProc)
  cmdChannel=make(chan CommandInfo,10)
  moduleSync=sync.RWMutex{}
  cmdProcSync=sync.RWMutex{}
  go execCmdThr()
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

// ------------------------ MODULE PRIVATE METHOD END ------------------------}}}

// ------------------------ MODULE PUBLIC METHOD ------------------------{{{

//Command process{{{

func AddCmddListener(proc TrackCmdProc) uuid.UUID_t {
  if proc == nil {return uuid.UUIDNull()}
  cmdProcSync.Lock()
  defer cmdProcSync.Unlock()
  procid := uuid.UUID1()
  tabCmdReader[procid] = proc
  return procid
}

func RemoveCmdListener(id uuid.UUID_t){
  cmdProcSync.Lock()
  defer cmdProcSync.Unlock()
  _,ok := tabCmdReader[id]
  if ok {
    delete(tabCmdReader,id)
  }
}

func SendCmd(cmd CommandInfo) {
  cmdChannel <- cmd
}

func execCmdThr(){
  for {
    cmd := <-cmdChannel
    func(){
      cmdProcSync.RLock()
      defer cmdProcSync.RUnlock()
      for _,vproc := range tabCmdReader {
        go vproc(cmd.Id,cmd.Cmd)
      }
    }()
  }
}

//}}}

//Status check{{{

//
func EnumTracks() []TrackInfo {
  trkchan := make(chan []TrackInfo)// {{{
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
  return <-trkchan// }}}
}

//
func GetTrackInfo(trackid uuid.UUID_t) *TrackInfo {
  trkchan := make(chan *TrackInfo)// {{{
  go readDecorator(func(){
    trk,ok := tabTrack[trackid]
    if ok {
      trkinf := TrackInfo{trk.id, trk.name, trk.bindA.GetID(), trk.bindB.GetID()}
      trkchan <- &trkinf
    }else{
      trkchan <- nil
    }
  })
  return <-trkchan// }}}
}

//
func EnumServices() []ServiceInfo {
  svrchan := make(chan []ServiceInfo)// {{{
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
  return <-svrchan// }}}
}

//
func GetServiceInfo(svrid uuid.UUID_t) *ServiceInfo {
  svrchan := make(chan *ServiceInfo)// {{{
  go readDecorator(func(){
    svr,ok := tabService[svrid]
    if ok {
      svrinf := ServiceInfo{svr.GetID(), svr.GetAddr()}
      svrchan <- &svrinf
    }else{
      svrchan <- nil
    }
  })
  return <-svrchan// }}}
}

//}}}

//
func BeginService(protocol string,addr string,sesLife *time.Duration) uuid.UUID_t {
  var nettype iohub.NetType// {{{
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
  if sesLife != nil {
    svr.SetDefaultLife(*sesLife)
  }
  svr.Start()
  return svr.GetID()// }}}
}

//
func QuickBindTrack(rule string,name string,servA uuid.UUID_t,servB uuid.UUID_t) uuid.UUID_t {
  var transBinder = MakeTrackBinder(nil)(nil)// {{{
  var c2m func (dstr TrackDistributer) TrackBinder = 
  MakeTrackBinder( func(frm *iohub.Frame_t) (out []*iohub.Frame_t, answ *iohub.Frame_t) {
      sendata := ModuleProtocolEnc(frm.ID,frm.Data,0)
      frmlist := make([]*iohub.Frame_t,len(sendata))
      for i,fr := range sendata{
        frmlist[i] = &(iohub.Frame_t{
          ID:uuid.UUIDNull(), Data:fr, Event:iohub.SEVT_BCAST, Err:frm.Err})
      }
      return frmlist,nil
  } )
  var m2c func (dstr TrackDistributer) TrackBinder = 
  MakeTrackBinder(func() func(frm *iohub.Frame_t) (out []*iohub.Frame_t, answ *iohub.Frame_t) {
    nextProcs := make(map[uuid.UUID_t]MPotoIter_t)
    return func(frm *iohub.Frame_t) (out []*iohub.Frame_t, answ *iohub.Frame_t) {
      if frm.Data != nil && len(frm.Data) > 0 {
        _,ok := nextProcs[frm.ID]
        if !ok {
          fstack,nextiter := ModuleProtocolEntry()(frm.Data)
          if nextiter == nil {
            return nil,&(iohub.Frame_t{ID:frm.ID,Data:nil,Event:iohub.SEVT_TERM})
          }
          if fstack != nil {
            var answ *iohub.Frame_t = nil
            allfrm := make([]*iohub.Frame_t,0,len(fstack))
            for i:=len(fstack); i > 0; i-- {
              perfrm := fstack[i-1]
              switch {
              case perfrm.tag == MPROTO_LAST:
                allfrm = append(allfrm,&(iohub.Frame_t{
                  ID:perfrm.cli_id,Data:perfrm.data,Event:iohub.SEVT_TERM,
                }))
              case perfrm.tag == MPROTO_TO && len(perfrm.data)>0:
                allfrm = append(allfrm,&(iohub.Frame_t{ID:perfrm.cli_id,Data:perfrm.data}))
              case perfrm.tag == MPROTO_TEST:
                answ = &(iohub.Frame_t{ID:frm.ID,Data:ModuleProtocolEnc(perfrm.cli_id,nil,MPROTO_TEST)[0]})
              }
            }
            return allfrm,answ
          }
        }
      }
      return nil,nil
    }
  }())
  switch {
  case rule == "T":   //Transparent broadcast
    return BindTrack(name,transBinder,transBinder,servA,servB)
  case rule == "C":   //Client to modlue-format
    return BindTrack(name,c2m(nil),m2c(nil),servA,servB)
  case rule == "D":   //Module-format distrabute broadcast
    //
  case rule[0:2] == "D*":  //Subscript module-format broadcast (buffered)
    //
  case rule[0:2] == "D#":  //Module-format hash distrabute (buffered)
    //
  }
  return uuid.UUIDNull()// }}}
}

//
func MakeTrackBinder(flt TrackFilter) func (dstr TrackDistributer) TrackBinder {
  return func (dstr TrackDistributer) TrackBinder{// {{{
    return func(distsvr,srcsvr iohub.Service_i) func(frm *iohub.Frame_t) {
      sendAllDist := func(outfrms []*iohub.Frame_t){
        if outfrms != nil {
          for _,perfrm := range outfrms { distsvr.Write(perfrm) }
        }
      }
      switch{
      case flt != nil && dstr != nil:
        return func(frm *iohub.Frame_t){
          out,answ := flt(frm)
          if answ != nil { srcsvr.Write(answ) }
          sendAllDist(dstr(out))
        }
      case dstr != nil:
        return func(frm *iohub.Frame_t){
          sendAllDist(dstr([]*iohub.Frame_t{frm}))
        }
      case flt != nil:
        return func(frm *iohub.Frame_t){
          out,answ := flt(frm)
          if answ != nil { srcsvr.Write(answ) }
          if out != nil {
            sendAllDist(out)
          }
        }
      default:  //
        return func (frm *iohub.Frame_t) {
          //Todo: output message log
          if frm != nil && len(frm.Data)>0 {
            distsvr.Write(&(iohub.Frame_t{ID:uuid.UUIDNull(),Data:frm.Data,Err:frm.Err,
              Event:iohub.SEVT_BCAST}))
          }
        } 
      }
    }
  }// }}}
}

//
func BindTrack(name string, procA,procB TrackBinder, servA,servB uuid.UUID_t) uuid.UUID_t {
  idchan := make(chan uuid.UUID_t)// {{{
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
    rdBinderA:procA, rdBinderB:procB}
    trk.bindFuncA = svrAObj.RegReader(procA(svrBObj,svrAObj))
    trk.bindFuncB = svrBObj.RegReader(procB(svrAObj,svrBObj))
    tabTrack[trk.id] = trk
    serviceRefAdd(servA)
    serviceRefAdd(servB)
    idchan <- trk.id
  })
  return <-idchan// }}}
}

//
func BindCallback(proc func(frm *iohub.Frame_t),serv uuid.UUID_t) uuid.UUID_t {
  idchan := make(chan uuid.UUID_t)// {{{
  go uniqueDecorator(func(){
    svrObj,ok := tabService[serv]
    if !ok {
      idchan <- uuid.UUIDNull()
      return
    }
    callid := svrObj.RegReader(proc)
    tabCallback[callid] = svrObj
    serviceRefAdd(serv)
    idchan <- callid
  })
  return <-idchan// }}}
}

//
func RemoveTrack(id uuid.UUID_t) {
  go uniqueDecorator(func(){// {{{
    trk,ok := tabTrack[id]
    if !ok {return}
    trk.bindA.UnregReader(trk.bindFuncA)
    serviceRefDec(trk.bindA.GetID())
    trk.bindB.UnregReader(trk.bindFuncB)
    serviceRefDec(trk.bindB.GetID())
    delete(tabTrack,id)
  })// }}}
}

//
func RemoveCall(id uuid.UUID_t) {
  go uniqueDecorator(func(){// {{{
    csvr,ok := tabCallback[id]
    if !ok {return}
    csvr.UnregReader(id)
    serviceRefDec(csvr.GetID())
    delete(tabCallback,id)
  })// }}}
}

//
func CloseService(id uuid.UUID_t) bool {
  oprst := make(chan bool)// {{{
  go uniqueDecorator(func(){
    svr,have := tabService[id]
    if have {
      _,noref := tabServiceRef[id]
      if !noref {
        svr.Terminate()
        oprst <- true
        return
      }
    }
    oprst <- false
  })
  return <- oprst// }}}
}

//
func WriteTo(service uuid.UUID_t,session uuid.UUID_t,data []byte) bool {
  oprst := make(chan bool)// {{{
  go readDecorator(func(){
    svr,ok := tabService[service]
    if !ok {
      oprst <- false
      return
    }
    var evt iohub.SessionEvent
    if session.IsNull() { evt=iohub.SEVT_BCAST }else{ evt=iohub.SEVT_DATA }
    oprst <- svr.Write(&(iohub.Frame_t{ID:session, Data:data, Event:evt}))
  })
  return <- oprst// }}}
}

// ------------------------ MODULE PUBLIC METHOD END ------------------------}}}

