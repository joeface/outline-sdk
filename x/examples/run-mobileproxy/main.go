// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/Jigsaw-Code/outline-sdk/x/mobileproxy"
)

// memoryStrategyCache implements mobileproxy.StrategyCache using an in-memory map
type memoryStrategyCache struct {
	mu   sync.RWMutex
	data map[string]string
}

// MemoryStrategyCache creates a new in-memory strategy cache
func MemoryStrategyCache() mobileproxy.StrategyCache {
	return &memoryStrategyCache{
		data: make(map[string]string),
	}
}

// Get retrieves the string value associated with the given key.
// Returns an empty string if the key is not found.
func (c *memoryStrategyCache) Get(key string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.data[key]
}

// Put adds the string value with the given key to the cache.
// If called with empty value, it removes the cache entry.
func (c *memoryStrategyCache) Put(key string, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if value == "" {
		delete(c.data, key)
	} else {
		c.data[key] = value
	}
}

func main() {
	transportFlag := flag.String("transport", "", "Transport config, e.g. ss:// or a direct json/yaml config")
	hostFlag := flag.String("domain", "", "Domain to find a strategy in case of smartFlag enabled")
	smartFlag := flag.String("smart", "", "Use SmartDialer functionality to find a strategy")
	cacheFlag := flag.String("cache", "", "Enable caching")
	measurableFlag := flag.String("measure", "", "Enable measuring")
	addrFlag := flag.String("localAddr", "localhost:8080", "Local proxy address")
	urlProxyPrefixFlag := flag.String("proxyPath", "/proxy", "Path where to run the URL proxy. Set to empty (\"\") to disable it.")
	flag.Parse()

	var dialer *mobileproxy.StreamDialer
	var err error

	if *smartFlag != "" {
		sdOptions := mobileproxy.NewSmartDialerOptions(mobileproxy.NewListFromLines(*hostFlag), *transportFlag)
		if *cacheFlag != "" {
			sdOptions.SetStrategyCache(MemoryStrategyCache())
		}

		dialer, err = sdOptions.NewStreamDialer()

	} else {
		dialer, err = mobileproxy.NewStreamDialerFromConfig(*transportFlag)
	}

	if err != nil {
		log.Fatalf("NewStreamDialerFromConfig failed: %v", err)
	}

	if *measurableFlag != "" {
		dialer.RunSpeedUpdater(1 * time.Second)
	}

	proxy, err := mobileproxy.RunProxy(*addrFlag, dialer)
	if err != nil {
		log.Fatalf("RunProxy failed: %v", err)
	}
	if *urlProxyPrefixFlag != "" {
		proxy.AddURLProxy(*urlProxyPrefixFlag, dialer)
	}

	log.Printf("TLS Transport: %s", dialer.TransportConfig)
	log.Printf("Proxy listening on %v", proxy.Address())

	timeStart := time.Now()

	if *measurableFlag != "" {
		go func() {
			for {
				time.Sleep(1 * time.Second)
				_, receiveMbps, _, totalReceived := dialer.GetSpeeds()
				fmt.Printf("--> %.2f Mbps | %.2f MB \n",
					receiveMbps,
					float64(totalReceived)/1_000_000)
			}
		}()
	}

	// Wait for interrupt signal to stop the proxy.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig
	log.Print("Shutting down")
	if *measurableFlag != "" {
		timeEnd := time.Now()
		duration := timeEnd.Sub(timeStart)
		fmt.Printf("\nSession Overview:\n Duration: %s", duration)
		_, _, finalTotalSent, finalTotalReceived := dialer.GetSpeeds()
		fmt.Printf("\n Sent: %.2f MB\n Received: %.2f MB\n Avg Speed: %.2f Mbps\n",
			float64(finalTotalSent)/1_000_000, float64(finalTotalReceived)/1_000_000, float64(finalTotalSent+finalTotalReceived)/1_000_000/duration.Seconds())
	}

	proxy.Stop(2)
}
