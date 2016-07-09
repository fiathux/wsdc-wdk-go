package iohub

import (
  "github.com/fiathux/wsdc-wdk-go/rfc/uuid"
  "net"
  "time"
  "sync"
  "os"
)

import "fmt"

const NETBUFFER_SIZE = 4096       //Max logic frame size
const NETBUFFER_CHANQUEUE = 50000 //Max service data-frame queue
const NETBUFFER_SESQUEUE = 20     //Max session data-frame queue
const NETBUFFER_DISTQUEUE = 100   //Max distibute data-frame queue
const DEFAULT_SESSION_LIFE = 30   //Default session life(second) while it not active
var creator map[NetType]func(hostaddress string)Service_i
var module_instance uuid.UUID_t;

// ------------------------ TYPE DEFINE ------------------------ {{{
//Basic data frame
type Frame_t struct {
  ID uuid.UUID_t
  Data []byte
  Err error
  Event SessionEvent
}

//Basic network session
type session_t struct {
  id uuid.UUID_t
  from uuid.UUID_t
  wBuffer chan Frame_t
  term chan bool
  sk net.Conn
  lifeInterval time.Duration
  life time.Time
  nettype NetType
}

//Stream session
type sessionStream_t struct {
  session_t
  rBuffer chan Frame_t
}

//Datagram session
type sessionDgram_t struct {
  session_t
  remoteAddr net.Addr
}

//Basic service struct
type service_t struct {
  id uuid.UUID_t
  readerLock,sessionLock *sync.RWMutex
  wBuffer,rBuffer chan Frame_t
  nettype NetType
  term chan bool
  defaultLifeInterval time.Duration
  children map[uuid.UUID_t]Session_i
  reader map[uuid.UUID_t] chan *Frame_t
  started bool
}

//Stream service class
type serviceStream_t struct {
  service_t
  sk net.Listener
}

//Packet service class
type servicePacket_t struct {
  service_t
  sk ListenerDgram_i
}

//Socket type enum
type NetType byte
const (
  NT_TCP NetType = iota
  NT_UNIX
  NT_UDP
  NT_UNIXPACK
)

type SessionEvent byte
const (
  SEVT_DATA    SessionEvent = iota  //Event data sendding or receiving
  SEVT_BCAST                        //Broadcast data
  SEVT_CONNECT                      //Event connecting
  SEVT_DOWN                         //Event disconnected
  SEVT_TEST                         //Event life test
  SEVT_TERM                         //Event send terminate data(send and close)
  SEVT_SENTFAL                      //Event failed data send
)
// ------------------------ TYPE DEFINE END ------------------------ }}}

// ------------------------ INTERFACES ------------------------ {{{

type ListenerDgram_i interface {
  net.Conn
  ReadFrom(b []byte) (int, net.Addr, error)
  WriteTo(b []byte, addr net.Addr) (int, error)
}

type Session_i interface {
  GetID() uuid.UUID_t
  GetAddr() string
  GetLife() time.Time
  getBody() *session_t
  ifOutDate() bool
  terminate()
}

type Service_i interface {
  Start()
  GetID() uuid.UUID_t
  GetAddr() string
  RegReader(ReaderProc func(*Frame_t)) (ReaderID uuid.UUID_t)
  UnregReader(ReaderID uuid.UUID_t)
  Write(DataFrame *Frame_t) bool
  KillSession(SessionID uuid.UUID_t)
  SetDefaultLife(interval time.Duration)
  UpdateSessionLife(SessionID uuid.UUID_t,life time.Time) bool
  HasSession(SessionID uuid.UUID_t) bool
  GetSession(SessionID uuid.UUID_t) (Session_i,bool)
  EnumSession() []Session_i
  Terminate()
  //Private:
  setSession(SessionObj Session_i) bool
  removeSession(SessionID uuid.UUID_t)
  clearSession()
}

// ------------------------ INTERFACES END ------------------------ }}}

// ------------------------ MODULE METHOD ------------------------ {{{

//Basic data-type support{{{

func getNetworkIDGenerator(proto NetType) func(uuid.UUID_t)func(string)uuid.UUID_t {
  var proto_head_s string
  switch proto {
    case NT_TCP:
      proto_head_s = "tcp://"
    case NT_UDP:
      proto_head_s = "udp://"
    case NT_UNIX:
      proto_head_s = "unix://"
    case NT_UNIXPACK:
      proto_head_s = "unix-packet://"
    default:
      return nil
  }
  return func(source_id uuid.UUID_t) func(string)uuid.UUID_t {
    proto_id_s := source_id.String()
    return func(addr string) uuid.UUID_t {
      return uuid.UUID5(uuid.NAMESPACE_URL, []byte(proto_head_s + proto_id_s + "@" + addr))
    }
  }
}

func fillBasicServiceStructure(s *service_t,addr string,proto NetType) {
  s.id = getNetworkIDGenerator(proto)(module_instance)(addr)
  s.readerLock = &(sync.RWMutex{})
  s.sessionLock = &(sync.RWMutex{})
  s.wBuffer = make(chan Frame_t,NETBUFFER_CHANQUEUE)
  s.rBuffer = make(chan Frame_t,NETBUFFER_CHANQUEUE)
  s.nettype = proto
  s.term = make(chan bool)
  s.defaultLifeInterval = DEFAULT_SESSION_LIFE * time.Second
  s.children = make(map[uuid.UUID_t]Session_i)
  s.reader = make(map[uuid.UUID_t]chan *Frame_t)
  s.started = false
}

func fillBasicSessionStructure(s Session_i,recv_id *uuid.UUID_t,
sk net.Conn,from uuid.UUID_t,defaultLife time.Duration,nettype NetType){
  ses := s.getBody()
  if recv_id != nil {
    ses.id = *recv_id
  }else{
    ses.id = uuid.UUID1()
  }
  ses.from = from
  ses.wBuffer = make(chan Frame_t,NETBUFFER_SESQUEUE)
  ses.term = make(chan bool)
  ses.lifeInterval = defaultLife
  if ses.lifeInterval != 0 {
    ses.life = time.Now().Add(ses.lifeInterval)
  }
  ses.sk = sk
  ses.nettype = nettype
}
//}}}

//Service distrabutor{{{

//Write distributor
func writeDistributor (service service_t){
  //fmt.Println("Start service W-DISTR")
  //defer fmt.Println("Stop service W-DISTR")
  for{
    select {
    case <-service.term:
        return
    case frm:=<-service.wBuffer:
      ses,_ := service.GetSession(frm.ID)
      if ses != nil {
        ses_t := ses.getBody()
        select {
        case ses_t.wBuffer <- frm:
        default:
          ses.terminate()
        }
      }
    }
  }
}

//Read distributor
func ReadDistributor (service service_t){
  //fmt.Println("Start service R-DISTR")
  //defer fmt.Println("Stop service R-DISTR")
  for{
    select {
    case <-service.term:
        return
    case frm:=<-service.rBuffer:
      //fmt.Println("Read distributing...")
      func (){
        service.readerLock.RLock()
        defer service.readerLock.RUnlock()
        for _,v := range service.reader {
          v<-&frm
        }
      }()
    }
  }
}
//}}}

//Stream acceptor {{{
func startStreamAcceptor(service *serviceStream_t,nettype NetType){
  go writeDistributor(service.service_t)
  go ReadDistributor(service.service_t)
  for {
    conn,err := service.sk.Accept()
    if err != nil {
      return
    }
    ses := sessionStream_t{}
    fillBasicSessionStructure(&ses,nil,conn,service.id,service.defaultLifeInterval,nettype)
    ses.rBuffer = make(chan Frame_t,NETBUFFER_SESQUEUE)
    service.setSession(&ses)
    // R/W Thread{{{
    go func(s *sessionStream_t){//Connecting event
      service.rBuffer <- Frame_t{ID:s.id,Data:nil,Err:nil,Event:SEVT_CONNECT}
    }(&ses)
    go func(s *sessionStream_t){//Reader
      //fmt.Println("Start session READER")
      //defer fmt.Println("Stop session READER")
      for{
        buf := [NETBUFFER_SIZE]byte{}
        recvlen,err := s.sk.Read(buf[:])
        //fmt.Println("Read data length",recvlen)
        if err!=nil || recvlen==0 {
          s.terminate()
          return
        }else{
          if s.lifeInterval != 0 {
            s.life = time.Now().Add(s.lifeInterval)
          }
          service.rBuffer <- Frame_t{ID:s.id,Data:buf[0:recvlen],Err:err,Event:SEVT_DATA}
        }
      }
    }(&ses)
    go func(s *sessionStream_t){//Writer
      //fmt.Println("Start session WRITER")
      //defer fmt.Println("Stop session WRITER")
      defer func(){
        fmt.Println("Close session",s.id)
        service.removeSession(s.GetID())
        for {
          select{
          case frm := <-s.wBuffer:
            service.rBuffer <- Frame_t{ID:s.id,Data:frm.Data,Err:err,Event:SEVT_SENTFAL}
          default:
            service.rBuffer <- Frame_t{ID:s.id,Data:nil,Err:err,Event:SEVT_DOWN}
            s.sk.Close()
            return
          }
        }
      }()
      for{
        select {
        case <-s.term:
            return
        case frm := <-s.wBuffer:
          if frm.Data != nil && len(frm.Data) > 0 {
            _,err := s.sk.Write(frm.Data)
            if err != nil {
              //Todo: report write error(if exist)
              //fmt.Println("Exit session [write error]",s.id)
              service.rBuffer <- Frame_t{ID:s.id,Data:frm.Data,Err:err,Event:SEVT_SENTFAL}
              return
            }
            if s.lifeInterval != 0 {
              s.life = time.Now().Add(s.lifeInterval)
            }
          }
          if frm.Event == SEVT_TERM || s.ifOutDate() {
            return
          }
        }
      }
    }(&ses)
    go func (s *sessionStream_t){//Timer
      for {
        select {
        case <-s.term:
          return
        default:
          s.wBuffer <- Frame_t{ID:s.id,Data:nil,Event:SEVT_TEST}
          time.Sleep(10*time.Second)
        }
      }
    }(&ses)
    //}}}
  }
}
//}}}

//Data-gram read/write process{{{
func startDgramRWProc(service *servicePacket_t,nettype NetType) {
  //Dgram session ID
  genRemoteID := getNetworkIDGenerator(nettype)(service.id)
  getRemoteID := func(addr net.Addr) uuid.UUID_t {
    if addr == nil {
      return uuid.UUIDNull()
    }
    addr_s := addr.String()
    if addr_s == "" {
      return uuid.UUIDNull()
    }
    return genRemoteID(addr_s)
  }
  //
  go writeDistributor(service.service_t)
  go ReadDistributor(service.service_t)
  for {
    buf := [NETBUFFER_SIZE]byte{}
    buflen,addr,err := service.sk.ReadFrom(buf[:])
    if err != nil {
      return
    }
    recvID := getRemoteID(addr)
    frm := Frame_t{ID:recvID,Data:buf[0:buflen]}
    rcvses,ok := service.GetSession(recvID)
    if !recvID.IsNull() && !ok {
      ses := sessionDgram_t{}
      fillBasicSessionStructure(&ses,&recvID,service.sk,service.id,
        service.defaultLifeInterval,nettype)
      ses.id = recvID
      ses.remoteAddr=addr
      service.setSession(&ses)
      frm.Event = SEVT_CONNECT
      //Writer {{{
      go func(s *sessionDgram_t) {
        //fmt.Println("Start Gram session WRITER")
        //defer fmt.Println("Stop Gram session WRITER")
        defer func(){
          fmt.Println("Close session",s.id)
          service.removeSession(s.GetID())
          for {
            select{
            case frm := <-s.wBuffer:
              service.rBuffer <- Frame_t{ID:s.id,Data:frm.Data,Err:err,Event:SEVT_SENTFAL}
            default:
              service.rBuffer <- Frame_t{ ID:recvID,Data:nil,Event:SEVT_DOWN }
              return
            }
          }
        }()
        for{
          select {
          case <-s.term:
              return
          case frm := <-s.wBuffer:
            if frm.Data != nil && len(frm.Data) > 0 {
              _,err := service.sk.WriteTo(frm.Data,s.remoteAddr)
              if err != nil {
                //Todo: report write error(if exist)
                //fmt.Println("Exit session [write error]",s.id)
                service.rBuffer <- Frame_t{ID:s.id,Data:frm.Data,Err:err,Event:SEVT_SENTFAL}
                return
              }
              if s.lifeInterval != 0 {
                s.life = time.Now().Add(s.lifeInterval)
              }
            }
            if frm.Event == SEVT_TERM || s.ifOutDate() {
              return
            }
          }
        }
      }(&ses)
      //}}}
      //Timer{{{
      go func (s *sessionDgram_t){
        for {
          select {
          case <-s.term:
            return
          default:
            s.wBuffer <- Frame_t{ID:s.id,Data:nil,Event:SEVT_TEST}
            time.Sleep(20*time.Second)
          }
        }
      }(&ses)
      //}}}
    }else if s := rcvses.getBody();s.lifeInterval != 0 {
      s.life = time.Now().Add(s.lifeInterval)
    }
    service.rBuffer <- frm
  }
}
//}}}

//Module inistialization
func init(){
  module_instance = uuid.UUID1()
  //Initialize service creator methods{{{
  creator = map[NetType]func(hostaddress string) Service_i {
    //TCP Service creator{{{
    NT_TCP:func (hostaddress string) Service_i {
      s := serviceStream_t{}
      fillBasicServiceStructure(&s.service_t,hostaddress,NT_TCP)
      var err error
      s.sk,err = net.Listen("tcp",hostaddress)
      if err != nil {
        return nil
      }
      return &s
    },
    //}}}
    //UDP Service creator{{{
    NT_UDP:func (hostaddress string) Service_i {
      s := servicePacket_t{}
      fillBasicServiceStructure(&s.service_t,hostaddress,NT_UDP)
      addr,err := net.ResolveUDPAddr("udp",hostaddress)
      if err != nil {
        return nil
      }
      s.sk,err = net.ListenUDP("udp",addr)
      if err != nil {
        return nil
      }
      return &s
    },
    //}}}
    //Unix stream socket Service creator{{{
    NT_UNIX:func (hostaddress string) Service_i {
      s := serviceStream_t{}
      fillBasicServiceStructure(&s.service_t,hostaddress,NT_UNIX)
      var err error
      s.sk,err = net.Listen("unix",hostaddress)
      if err != nil {
        return nil
      }
      return &s
    },
    //}}}
    //Unix datagram socket Service creator{{{
    NT_UNIXPACK:func (hostaddress string) Service_i {
      s := servicePacket_t{}
      fillBasicServiceStructure(&s.service_t,hostaddress,NT_UNIXPACK)
      addr,err := net.ResolveUnixAddr("unixgram",hostaddress)
      if err != nil {
        return nil
      }
      s.sk,err = net.ListenUnixgram("unixgram",addr)
      if err != nil {
        return nil
      }
      return &s
    },
    //}}}
  }
  //}}}
}

//Get module instance ID
func GetModuleID() uuid.UUID_t {
  return module_instance
}

// ------------------------ MODULE METHOD END ------------------------ }}}

// ------------------------ CLASS METHOD ------------------------ {{{

//Basic service method{{{

func (s *service_t) GetID() uuid.UUID_t {
  return s.id
}

func (s *service_t) RegReader(ReaderProc func(*Frame_t)) (ReaderID uuid.UUID_t) {
  s.readerLock.Lock()
  defer s.readerLock.Unlock()
  readerID := uuid.UUID1()
  fmchan := make(chan *Frame_t,NETBUFFER_DISTQUEUE)
  s.reader[readerID] = fmchan
  go func (){
    for{
      frm,ok := <-fmchan
      if !ok {
        return
      }
      ReaderProc(frm)
    }
  }()
  return readerID
}

func (s *service_t) UnregReader(ReaderID uuid.UUID_t) {
  s.readerLock.Lock()
  defer s.readerLock.Unlock()
  delete(s.reader,ReaderID)
}

func (s *service_t) Write(DataFrame *Frame_t) bool {
  s.sessionLock.RLock()
  defer s.sessionLock.RUnlock()
  if (DataFrame.ID.IsNull() || DataFrame.ID == s.id) && DataFrame.Event == SEVT_BCAST {
    for _,i := range s.children {
      s.wBuffer <- Frame_t{ID:i.GetID(), Data:DataFrame.Data}
    }
    return true
  }else{
    _,ok := s.children[DataFrame.ID]
    if ok {
      s.wBuffer <- *DataFrame
    }
    return ok
  }
}

func (s *service_t) SetDefaultLife(interval time.Duration) {
  s.defaultLifeInterval = interval
}

func (s *service_t) UpdateSessionLife(SessionID uuid.UUID_t,life time.Time) bool {
  s.sessionLock.RLock()
  defer s.sessionLock.RUnlock()
  ses,ok := s.children[SessionID]
  if ok {
    ses_t := ses.getBody()
    ses_t.life = life
  }
  return ok
}

func (s *service_t) HasSession(SessionID uuid.UUID_t) bool {
  s.sessionLock.RLock()
  defer s.sessionLock.RUnlock()
  _,ok := s.children[SessionID]
  return ok
}

func (s *service_t) CountSession() int {
  s.sessionLock.RLock()
  defer s.sessionLock.RUnlock()
  return len(s.children)
}

func (s *service_t) GetSession(SessionID uuid.UUID_t) (Session_i,bool) {
  s.sessionLock.RLock()
  defer s.sessionLock.RUnlock()
  ses,ok := s.children[SessionID]
  return ses,ok
}

func (s *service_t) setSession(SessionObj Session_i) bool {
  s.sessionLock.Lock()
  defer s.sessionLock.Unlock()
  ses_id := SessionObj.getBody().id
  _,ok := s.children[ses_id]
  if !ok {
    s.children[ses_id] = SessionObj
    return true
  }else{
    return false
  }
}

func (s *service_t) removeSession(SessionID uuid.UUID_t) {
  s.sessionLock.Lock()
  defer s.sessionLock.Unlock()
  _,ok := s.children[SessionID]
  if ok {
    delete(s.children,SessionID)
    //fmt.Println("Remove session: ",SessionID," now keep:",len(s.children))
  }
}

func (s *service_t) clearSession(){
  s.sessionLock.Lock()
  defer s.sessionLock.Unlock()
  for _,v :=range s.children {
    go func(ses Session_i){
      ses.terminate()
    }(v)
  }
}

func (s *service_t) KillSession(SessionID uuid.UUID_t) {
  s.sessionLock.RLock()
  defer s.sessionLock.RUnlock()
  ses,ok := s.children[SessionID]
  if ok {
    go ses.terminate()
  }
}

func (s *service_t) EnumSession() []Session_i {
  s.sessionLock.RLock()
  defer s.sessionLock.RUnlock()
  rst := make([]Session_i,0,len(s.children))
  for _,v :=range s.children {
    rst = append(rst,v)
  }
  return rst
}

func (s *service_t) Terminate() {
  s.clearSession()
  for {
    if len(s.children) == 0{
      break
    }
    time.Sleep(100*time.Millisecond)
  }
  //fmt.Println("Service session terminate [OK]")
  close(s.term)
  //fmt.Println("Service has been terminated!")
}
//}}}

//Overrided service method {{{

func (s *serviceStream_t) Start(){
  s.sessionLock.Lock()
  defer s.sessionLock.Unlock()
  if s.started { return }
  s.started = true
  go startStreamAcceptor(s,s.nettype)
}

func (s *serviceStream_t) Terminate() {
  s.sk.Close()
  if s.nettype == NT_UNIX {
    os.Remove(s.sk.Addr().String())
  }
  s.service_t.Terminate()
}

func (s *serviceStream_t) GetAddr() string {
  return s.sk.Addr().String()
}

func (s *servicePacket_t) Start(){
  s.sessionLock.Lock()
  defer s.sessionLock.Unlock()
  if s.started { return }
  s.started = true
  go startDgramRWProc(s,s.nettype)
}

func (s *servicePacket_t) Terminate() {
  s.sk.Close()
  if s.nettype == NT_UNIXPACK {
    os.Remove(s.sk.LocalAddr().String())
  }
  s.service_t.Terminate()
}

func (s *servicePacket_t) GetAddr() string {
  return s.sk.LocalAddr().String()
}

//}}}

//Session method {{{

func (s *session_t) GetID() uuid.UUID_t {
  return s.id;
}

func (s *session_t) GetLife() time.Time {
  return s.life
}

func (s *session_t) terminate() {
  close(s.term)
}

func (s *session_t) ifOutDate() bool {
  return !s.life.IsZero() && time.Now().Sub(s.life) > 0
}

func (s *sessionStream_t) getBody() *session_t {
  return &s.session_t;
}

func (s *sessionStream_t) GetAddr() string {
  return s.sk.RemoteAddr().String();
}

func (s *sessionDgram_t) getBody() *session_t {
  return &s.session_t;
}

func (s *sessionDgram_t) GetAddr() string {
  return s.sk.RemoteAddr().String();
}
//}}}

// ------------------------ CLASS METHOD END ------------------------ }}}

// ------------------------ PUBLIC METHOD ------------------------ {{{

func CreateService(proto NetType,hostaddress string) Service_i {
  return creator[proto](hostaddress)
}

//------------------------ PUBLIC METHOD END ------------------------}}}
