//go:build darwin && arm64 && cgo

package localname

/*
// macOS exports DNSService APIs through the default libSystem linkage.
// Modern SDKs do not provide a separately linkable libdns_sd stub.
#include <dns_sd.h>
#include <stdlib.h>
#include <stdint.h>
#include <poll.h>
#include <arpa/inet.h>
typedef struct {
 DNSServiceRef service;
 DNSRecordRef record;
 DNSRecordRef ipv6;
 DNSRecordRef ownership;
 int ready;
 DNSServiceErrorType error;
} aios_record;
static void aios_reply(DNSServiceRef service, DNSRecordRef record, DNSServiceFlags flags,
 DNSServiceErrorType error, void *context) {
 aios_record *r=(aios_record*)context;
 if (error) {r->ready=3;r->error=error;} else {r->ready++;}
}
static aios_record *aios_register(const char *host, const void *owner, uint16_t owner_size) {
 aios_record *r=calloc(1,sizeof(aios_record));
 if (!r) return NULL;
 r->error=DNSServiceCreateConnection(&r->service);
 if (r->error) {r->ready=3;return r;}
 struct in_addr address; inet_pton(AF_INET,"127.0.0.1",&address);
 r->error=DNSServiceRegisterRecord(r->service,&r->record,kDNSServiceFlagsUnique,
 kDNSServiceInterfaceIndexLocalOnly,host,kDNSServiceType_A,kDNSServiceClass_IN,
 sizeof(address),&address,120,aios_reply,r);
 // Both address families reach loopback sockets on the same explicit port.
 // Real ::1 also supports system clients that reject mapped AAAA addresses.
 struct in6_addr mapped; inet_pton(AF_INET6,"::1",&mapped);
 if (!r->error) r->error=DNSServiceRegisterRecord(r->service,&r->ipv6,kDNSServiceFlagsUnique,
 kDNSServiceInterfaceIndexLocalOnly,host,kDNSServiceType_AAAA,kDNSServiceClass_IN,
 sizeof(mapped),&mapped,120,aios_reply,r);
 if (!r->error) r->error=DNSServiceRegisterRecord(r->service,&r->ownership,kDNSServiceFlagsUnique,
 kDNSServiceInterfaceIndexLocalOnly,host,kDNSServiceType_TXT,kDNSServiceClass_IN,
 owner_size,owner,120,aios_reply,r);
 if (r->error) r->ready=3;
 return r;
}
static void aios_poll(aios_record *r) {
 if (!r->service || r->error) return;
 struct pollfd fd={DNSServiceRefSockFD(r->service),POLLIN,0};
 int result=poll(&fd,1,100);
 if (result>0) {
   if (fd.revents&POLLIN) {
    DNSServiceErrorType e=DNSServiceProcessResult(r->service);
    if (e) {r->error=e;r->ready=3;}
   } else if (fd.revents&(POLLERR|POLLHUP|POLLNVAL)) {r->error=kDNSServiceErr_Unknown;r->ready=3;}
 }
}
static void aios_free(aios_record *r) { if(r->service) DNSServiceRefDeallocate(r->service);free(r); }
*/
import "C"
import (
	"crypto/rand"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

type record struct {
	mu         sync.Mutex
	handle     *C.aios_record
	stop, done chan struct{}
	once       sync.Once
	errorCode  atomic.Int64
}

func registerNative(host string) (nativeRecord, error) {
	name := C.CString(host + ".")
	defer C.free(unsafe.Pointer(name))
	// A public random ownership TXT distinguishes identical loopback A records
	// from another login user; a collision must fail rather than silently rename.
	var nonce [16]byte
	if _, e := rand.Read(nonce[:]); e != nil {
		return nil, e
	}
	owner := []byte(fmt.Sprintf("aios-owner=%x", nonce))
	txt := append([]byte{byte(len(owner))}, owner...)
	value := C.CBytes(txt)
	defer C.free(value)
	h := C.aios_register(name, value, C.uint16_t(len(txt)))
	if h == nil {
		return nil, fmt.Errorf("DNS-SD allocation unavailable")
	}
	r := &record{handle: h, stop: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer close(r.done)
		for {
			select {
			case <-r.stop:
				r.mu.Lock()
				C.aios_free(r.handle)
				r.handle = nil
				r.mu.Unlock()
				return
			default:
				r.mu.Lock()
				C.aios_poll(r.handle)
				failed := r.handle.error != 0
				r.errorCode.Store(int64(r.handle.error))
				r.mu.Unlock()
				if failed {
					select {
					case <-r.stop:
					case <-time.After(100 * time.Millisecond):
					}
				}
			}
		}
	}()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		ready := r.handle.ready >= 3
		code := int(r.handle.error)
		r.mu.Unlock()
		if ready {
			if code != 0 {
				r.Close()
				return nil, fmt.Errorf("DNS-SD refused selected name (%d)", code)
			}
			return r, nil
		}
		time.Sleep(25 * time.Millisecond)
	}
	r.Close()
	return nil, fmt.Errorf("DNS-SD registration timed out")
}
func (r *record) Health() error {
	if r.errorCode.Load() != 0 {
		return fmt.Errorf("selected native namespace is unavailable")
	}
	return nil
}
func (r *record) Close() {
	r.once.Do(func() { r.errorCode.Store(-1); close(r.stop); <-r.done; r.errorCode.Store(-1) })
}
