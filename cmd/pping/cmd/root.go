package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/wzv5/pping/pkg/pping"

	"github.com/spf13/cobra"
)

var rootCmd *cobra.Command

type globalFlags struct {
	v    bool
	t    bool
	n    int
	i    time.Duration
	ipv4 bool
	ipv6 bool
}

var globalflag globalFlags

func init() {
	rootCmd = &cobra.Command{Use: filepath.Base(os.Args[0])}
	rootCmd.Version = "2020.07.09"
	rootCmd.PersistentFlags().BoolVarP(&globalflag.t, "infinite", "t", false, "ping the specified target until stopped")
	rootCmd.PersistentFlags().IntVarP(&globalflag.n, "count", "c", 4, "number of requests to send")
	rootCmd.PersistentFlags().DurationVarP(&globalflag.i, "interval", "i", time.Second*1, "delay between each request")
	rootCmd.PersistentFlags().BoolVarP(&globalflag.ipv4, "ipv4", "4", false, "use IPv4")
	rootCmd.PersistentFlags().BoolVarP(&globalflag.ipv6, "ipv6", "6", false, "use IPv6")

	rootCmd.PersistentPreRun = func(*cobra.Command, []string) {
		if globalflag.ipv4 && !globalflag.ipv6 {
			pping.LookupFunc = pping.LookupIPv4
		} else if !globalflag.ipv4 && globalflag.ipv6 {
			pping.LookupFunc = pping.LookupIPv6
		} else {
			pping.LookupFunc = pping.LookupIP
		}
	}

	addTcpCommand()
	addTlsCommand()
	addHttpCommand()
}

func Execute() error {
	return rootCmd.Execute()
}

func PingToChan(ctx context.Context, ping pping.IPing) <-chan pping.IPingResult {
	c := make(chan pping.IPingResult)
	go func() {
		c <- ping.PingContext(ctx)
	}()
	return c
}

func RunPing(ping pping.IPing) {
	// 预热，由于某些资源需要初始化，首次运行会耗时较长
	ping.Ping()

	s := statistics{}
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	ctx, cancel := context.WithCancel(context.Background())

	for i := 1; i <= globalflag.n || globalflag.t; i++ {
		select {
		case result := <-PingToChan(ctx, ping):
			PrintResult(i, result)
			s.append(result)
		case <-c:
			goto end
		}

		// 最后一次 ping 结束后不再等待
		if i == globalflag.n && !globalflag.t {
			break
		}

		select {
		case <-c:
			goto end
		case <-time.After(globalflag.i):
		}

		// 再次检查是否停止，上面的检查可能由于延迟为0而始终无法停止
		select {
		case <-c:
			goto end
		default:
		}
	}

end:
	cancel()
	s.print()
}

type statistics struct {
	max, min, total  int
	sent, ok, failed int
}

func (s *statistics) append(result pping.IPingResult) {
	if result == nil {
		return
	}
	s.sent++
	if result.Error() != nil {
		s.failed++
		return
	}
	t := result.Result()
	if s.ok == 0 {
		s.min = t
		s.max = t
	} else {
		if t < s.min {
			s.min = t
		} else if t > s.max {
			s.max = t
		}
	}
	s.total += t
	s.ok++
}

func (s *statistics) clear() {
	s.max = 0
	s.min = 0
	s.total = 0
	s.sent = 0
	s.ok = 0
	s.failed = 0
}

func (s *statistics) print() {
	if s.sent == 0 {
		return
	}
	fmt.Println()
	fmt.Printf("\tsent = %d, ok = %d, failed = %d (%d%%)\n", s.sent, s.ok, s.failed, 100*s.failed/s.sent)
	if s.ok > 0 {
		fmt.Printf("\tmin = %d ms, max = %d ms, avg = %d ms\n", s.min, s.max, s.total/s.ok)
	}
}

func PrintResult(i int, r pping.IPingResult) {
	log.Printf("[%d] %v\n", i, r)
}
