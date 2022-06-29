package main

import (
	"fmt"
	"github.com/gogf/gf/os/gcfg"
	"github.com/gogf/gf/os/glog"
	"github.com/osgochina/dmicro/drpc"
	"github.com/osgochina/dmicro/easyservice"
	"github.com/vprix/vncproxy/rfb"
	"github.com/vprix/vncproxy/security"
	"github.com/vprix/vncproxy/session"
	"github.com/vprix/vncproxy/vnc"
	"io"
	"net"
	"time"
)

// TcpSandBox  Tcp的服务
type TcpSandBox struct {
	id      int
	name    string
	cfg     *gcfg.Config
	service *easyservice.EasyService
	lis     net.Listener
	closed  chan struct{}
}

// NewTcpSandBox 创建一个默认的服务沙盒
func NewTcpSandBox(cfg *gcfg.Config) *TcpSandBox {
	id := easyservice.GetNextSandBoxId()
	sBox := &TcpSandBox{
		id:     id,
		name:   fmt.Sprintf("tcp_%d", id),
		cfg:    cfg,
		closed: make(chan struct{}),
	}
	return sBox
}

func (that *TcpSandBox) ID() int {
	return that.id
}

func (that *TcpSandBox) Name() string {
	return that.name
}

func (that *TcpSandBox) Setup() error {
	var err error
	addr := fmt.Sprintf("%s:%d", that.cfg.GetString("tcpHost"), that.cfg.GetInt("tcpPort"))
	that.lis, err = net.Listen("tcp", addr)
	if err != nil {
		glog.Fatalf("Error listen. %v", err)
	}
	fmt.Printf("Tcp proxy started! listening %s . vnc server %s:%d\n", that.lis.Addr().String(), that.cfg.GetString("vncHost"), that.cfg.GetInt("vncPort"))
	securityHandlers := []rfb.ISecurityHandler{
		&security.ServerAuthNone{},
	}
	if len(that.cfg.GetBytes("proxyPassword")) > 0 {
		securityHandlers = append(securityHandlers, &security.ServerAuthVNC{Password: that.cfg.GetBytes("proxyPassword")})
	}
	targetCfg := rfb.TargetConfig{
		Host:     that.cfg.GetString("vncHost"),
		Port:     that.cfg.GetInt("vncPort"),
		Password: that.cfg.GetBytes("vncPassword"),
	}
	for {
		conn, err := that.lis.Accept()
		if err != nil {
			select {
			case <-that.closed:
				return drpc.ErrListenClosed
			default:
			}
			return err
		}
		svrSess := session.NewServerSession(
			rfb.OptDesktopName([]byte("Vprix VNC Proxy")),
			rfb.OptHeight(768),
			rfb.OptWidth(1024),
			rfb.OptSecurityHandlers(securityHandlers...),
			rfb.OptGetConn(func(sess rfb.ISession) (io.ReadWriteCloser, error) {
				return conn, nil
			}),
		)
		timeout := 10 * time.Second
		network := "tcp"
		cliSess := session.NewClient(
			rfb.OptSecurityHandlers([]rfb.ISecurityHandler{&security.ClientAuthVNC{Password: targetCfg.Password}}...),
			rfb.OptGetConn(func(sess rfb.ISession) (io.ReadWriteCloser, error) {
				return net.DialTimeout(network, targetCfg.Addr(), timeout)
			}),
		)
		p := vnc.NewVncProxy(cliSess, svrSess)
		p.Start()
		for {
			err = <-p.Error()
			glog.Warning(err)
		}
	}
}

func (that *TcpSandBox) Shutdown() error {
	close(that.closed)
	return that.lis.Close()
}

func (that *TcpSandBox) Service() *easyservice.EasyService {
	return that.service
}
