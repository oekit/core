package lifecycle

import (
	"os"
	"syscall"
	"testing"
)

// TestDefaultListenSignals 钉住 core#231：Listen() 无参默认必须同时捕获
// SIGINT（Ctrl-C）与 SIGTERM（K8s/容器编排器的标准终止信号）。修复前只捕
// os.Interrupt，K8s 下 SIGTERM 直接按默认行为终止进程，优雅关停（drain /
// 反注册 / flush）完全不执行。
func TestDefaultListenSignals(t *testing.T) {
	signals := defaultListenSignals()
	hasInterrupt, hasTerm := false, false
	for _, sig := range signals {
		switch sig {
		case os.Interrupt:
			hasInterrupt = true
		case syscall.SIGTERM:
			hasTerm = true
		}
	}
	if !hasInterrupt {
		t.Errorf("default signals %v must contain os.Interrupt (Ctrl-C)", signals)
	}
	if !hasTerm {
		t.Errorf("default signals %v must contain syscall.SIGTERM (K8s graceful termination)", signals)
	}
	if len(signals) != 2 {
		t.Errorf("default signals = %v, want exactly {os.Interrupt, syscall.SIGTERM}", signals)
	}
}

// TestDefaultListenSignals_FreshSlice 钉住每次返回新切片：调用方改写不得
// 污染后续 Listen 的默认监听集合。
func TestDefaultListenSignals_FreshSlice(t *testing.T) {
	first := defaultListenSignals()
	first[0] = syscall.SIGKILL
	second := defaultListenSignals()
	if second[0] != os.Interrupt {
		t.Errorf("defaultListenSignals returned shared slice: second[0] = %v", second[0])
	}
}
