package kotomiMsg

import (
  "github.com/fiathux/wsdc-wdk-go/rfc/uuid"
  "encoding/binary"
  "time"
)

// ------------------------ TYPE DEFINE ------------------------{{{

type mPotoFrame_t struct {
  cli_id uuid.UUID_t
  frm_ts int64
  tag MPotoTag_t
  data []byte
}

type MPotoIter_t func(data []byte)(frmstack []mPotoFrame_t,nextparse MPotoIter_t)
type MPotoTag_t uint
const (
  MPROTO_FROM MPotoTag_t = iota //General data receive from client
  MPROTO_TO           //General data send to client
  MPROTO_CONNECT      //Client: Connect
  MPROTO_DISCONNECT   //Client: Disconnect
  MPROTO_LAST         //Module: Write last frame and require close this client
  MPROTO_TEST         //Module: Link test
)
const MPROTO_FRAMELEN_MAX = 65536 //Max module fram length
const MPROTO_STACK_CAP = 4
const ( //Module protocol head position
  MPH_L_MAGIC = 4
  MPH_L_CLIID = 16
  MPH_L_FRMID = 8
  MPH_L_SUBS = 4
  MPH_LEN = MPH_L_MAGIC + MPH_L_CLIID + MPH_L_FRMID + MPH_L_SUBS
)
const MPROTO_BODY_LEN int = MPROTO_FRAMELEN_MAX - MPH_LEN
const MPROTO_MAGIC string = "TSP:"

// ------------------------ TYPE DEFINE END ------------------------}}}

// ------------------------ MODULE PUBLIC METHOD ------------------------{{{
//Encode date to module frame data
func ModuleProtocolEnc(id uuid.UUID_t,data []byte,tag MPotoTag_t) [][]byte {
  result := make([][]byte,0,MPROTO_STACK_CAP)
  frm := mPotoFrame_t{ cli_id:id }
  if tag == MPROTO_CONNECT { frm.tag = tag } //"CONNECT" send at first frame
  for {
    frm.frm_ts = time.Now().UnixNano()
    if len(data) < (MPROTO_FRAMELEN_MAX - MPH_LEN) {
      if tag == MPROTO_DISCONNECT { //"DISCONNECT" send at last frame
        frm.tag = tag
      }
      if len(data) > 0 {
        frm.data = data
      }else{
        frm.data = nil
      }
      return append(result,frm.toBytes())
    }else{
      cutlen := MPROTO_FRAMELEN_MAX - MPH_LEN
      frm.data = data[0:cutlen]
      data = data[cutlen:]
      frm.tag = 0
      result = append(result,frm.toBytes())
    }
  }
}

//Create module protocol parser{{{
//IMPORTANT: parse result is frames STACK strature, the head item is last one
func ModuleProtocolEntry() MPotoIter_t {
  frmbuff := make([]byte,0,MPROTO_FRAMELEN_MAX)
  bodylen := 0
  pf := mPotoFrame_t{}
  var headParser MPotoIter_t
  var bodyParser MPotoIter_t
  //Head
  //Todo: change protocol parser for new protocol
  headParser = func(data []byte) ([]mPotoFrame_t,MPotoIter_t) {
    frmbuff = append(frmbuff,data...)
    if len(frmbuff) < MPH_LEN { 
      return nil,headParser
    }else{
      if string(frmbuff[0:MPH_L_MAGIC]) != MPROTO_MAGIC {
        return nil,nil
      }
      clipbuff := frmbuff[MPH_L_MAGIC:]
      copy(pf.cli_id[:],clipbuff[0:MPH_L_CLIID])
      clipbuff = frmbuff[MPH_L_CLIID:]
      pf.frm_ts = int64(binary.BigEndian.Uint64(clipbuff[0:MPH_L_FRMID]))
      clipbuff = frmbuff[MPH_L_FRMID:]
      inflen := binary.BigEndian.Uint32(clipbuff[0:MPH_L_SUBS])
      pf.tag = MPotoTag_t(inflen>>28)
      bodylen = int(inflen & 0x0fffffff)
      if bodylen > MPROTO_BODY_LEN{
        return nil,nil
      }
      return bodyParser(nil)
    }
  }
  //Body
  bodyParser = func(data []byte) ([]mPotoFrame_t,MPotoIter_t){
    if len(data) > 0 { frmbuff = append(frmbuff,data...) }
    if len(frmbuff[MPH_LEN:]) < bodylen { return nil,bodyParser }
    bdata := data[0:bodylen]
    data = data[bodylen:]
    stackFrm,flowPars := ModuleProtocolEntry()(data)
    pf.data = bdata
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

// ------------------------ MODULE PUBLIC METHOD END ------------------------}}}

// ------------------------ CLASS METHOD ------------------------{{{

func (f mPotoFrame_t) toBytes() []byte {
  frmlen := len(f.data) + MPH_LEN
  buff := make([]byte,MPH_LEN,frmlen)
  copy(buff[0:MPH_L_MAGIC],[]byte(MPROTO_MAGIC))
  clipFrm := buff[MPH_L_MAGIC:]  //>> MPH_L_MAGIC
  copy(clipFrm[0:MPH_L_CLIID],f.cli_id[:])
  clipFrm = clipFrm[MPH_L_CLIID:]   //>> MPH_L_CLIID
  binary.BigEndian.PutUint64(clipFrm[0:MPH_L_FRMID],uint64(f.frm_ts))
  clipFrm = clipFrm[MPH_L_FRMID:]   //>> MPH_L_FRMID
  bodyTagLen := uint32(len(f.data)) | (uint32(f.tag) << 28)
  binary.BigEndian.PutUint32(clipFrm[0:MPH_L_SUBS],bodyTagLen)
  if len(f.data) > 0 {
    return append(buff,f.data...)
  }else {
    return buff
  }
}

// ------------------------ CLASS METHOD END ------------------------}}}
