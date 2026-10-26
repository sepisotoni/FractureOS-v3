/* Fresh v3 diagnostic. No display/input ownership changes or control writes.
 * Barcelona ioctl declarations come from the matching local kernel headers.
 * Compile against the legacy toolchain libc; do not copy a modern libc to GPS.
 */
#define _GNU_SOURCE
#include <errno.h>
#include <fcntl.h>
#include <poll.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <unistd.h>
#include <sys/ioctl.h>
#include <sys/mman.h>
#include <sys/socket.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <asm/unistd.h>
#include <linux/fb.h>
#include <linux/input.h>
#include <linux/rtc.h>
#include <barcelona/Barc_Gpio.h>
#include <barcelona/Barc_pwm.h>
#include <barcelona/Barc_Battery.h>

/* GCC 3.3 sysroot predates clock_gettime; number is from the running 2.6.13
 * kernel include/asm-arm/unistd.h. Use its native OABI syscall trampoline. */
#ifndef __NR_clock_gettime
#define __NR_clock_gettime (__NR_SYSCALL_BASE + 263)
#endif

static int failures;
static void result(const char *name, int ok)
{
    printf("%s %s\n", ok ? "PASS" : "FAIL", name);
    if (!ok) failures++;
}

static void unavailable(const char *name)
{
    printf("UNAVAILABLE %s errno=%d (%s)\n", name, errno, strerror(errno));
}

static int export_hex(const char *path, unsigned long offset, unsigned long count)
{
    struct stat st;
    unsigned char buf[4096];
    char encoded[8193];
    const char digits[]="0123456789abcdef";
    size_t total = 0;
    ssize_t n, i;
    int fd = open(path, O_RDONLY | O_NONBLOCK);
    if (fd < 0) { perror(path); return 1; }
    if (fstat(fd, &st) || !S_ISREG(st.st_mode) || st.st_size > 8 * 1024 * 1024 || offset > (unsigned long)st.st_size) {
        fprintf(stderr, "Export requires a regular file of at most 8 MiB\n");
        close(fd); return 1;
    }
    if (lseek(fd, offset, SEEK_SET) < 0) { close(fd); return 1; }
    n=0;
    while (total < count && (n = read(fd, buf, count-total<sizeof buf?count-total:sizeof buf)) > 0) {
        total += n;
        if (total > 8 * 1024 * 1024) { close(fd); return 1; }
        for (i = 0; i < n; i++) { encoded[2*i]=digits[buf[i]>>4]; encoded[2*i+1]=digits[buf[i]&15]; }
        encoded[2*n]='\n';
        if (fwrite(encoded,1,2*n+1,stdout)!=(size_t)(2*n+1)){close(fd);return 1;}
    }
    close(fd);
    return n < 0 || ferror(stdout) ? 1 : 0;
}

static int probe(void)
{
    int fd, level;
    struct fb_fix_screeninfo fix;
    struct fb_var_screeninfo var;
    struct rtc_time rtc;
    GPIO_BUTTON_EVENT button;
    BATTERY_STATUS battery;
    /* 2.4 sysroot declares EVIOCGABS as int[5]; 2.6.13 uses the same five
     * integer layout (value, minimum, maximum, fuzz, flat). */
    int absinfo[5];
    char name[128];
    unsigned char *map;
    unsigned long sum = 0;
    size_t i;
    printf("FRACTURE_PLATFORM_PROBE_V1 uid=%lu long=%lu time_t=%lu input_event=%lu\n",
           (unsigned long)getuid(), (unsigned long)sizeof(long),
           (unsigned long)sizeof(time_t), (unsigned long)sizeof(struct input_event));
    fd = open("/dev/fb", O_RDONLY);
    if (fd < 0) unavailable("framebuffer.open");
    else {
        memset(&fix, 0, sizeof fix); memset(&var, 0, sizeof var);
        if (ioctl(fd, FBIOGET_FSCREENINFO, &fix) || ioctl(fd, FBIOGET_VSCREENINFO, &var))
            unavailable("framebuffer.info");
        else {
            printf("FB visible=%ux%u virtual=%ux%u bpp=%u stride=%u bytes=%u offsets=%u,%u rgb=%u/%u,%u/%u,%u/%u\n",
                   var.xres,var.yres,var.xres_virtual,var.yres_virtual,var.bits_per_pixel,
                   fix.line_length,fix.smem_len,var.xoffset,var.yoffset,
                   var.red.length,var.red.offset,var.green.length,var.green.offset,var.blue.length,var.blue.offset);
            if (fix.smem_len > 0 && fix.smem_len <= 1024 * 1024) {
                map = mmap(NULL, fix.smem_len, PROT_READ, MAP_SHARED, fd, 0);
                if (map == MAP_FAILED) unavailable("framebuffer.readonly_mmap");
                else {
                    for (i=0; i<fix.smem_len; i+=4096) sum += map[i];
                    printf("PASS framebuffer.readonly_mmap sample_sum=%lu\n",sum);
                    munmap(map,fix.smem_len);
                }
            }
        }
        close(fd);
    }
    fd = open("/dev/input/event0", O_RDONLY | O_NONBLOCK);
    if (fd < 0) unavailable("touch.open");
    else {
        memset(name,0,sizeof name);
        if (ioctl(fd,EVIOCGNAME(sizeof name - 1),name) >= 0) printf("TOUCH name=%s\n",name);
        else unavailable("touch.name");
        if (ioctl(fd,EVIOCGABS(ABS_PRESSURE),&absinfo) == 0)
            printf("TOUCH pressure=%d min=%d max=%d (coordinates ignored)\n",absinfo[0],absinfo[1],absinfo[2]);
        else unavailable("touch.pressure");
        close(fd);
    }
    fd = open("/dev/hwstatus", O_RDONLY | O_NONBLOCK);
    if (fd < 0) unavailable("button.open");
    else {
        /* Unlike IOR_HWSTATUS, this does not clear the driver's statusChanged. */
        if (ioctl(fd,IOR_BUTTON_EVENT,&button) == 0)
            printf("BUTTON last_sequence=%u last_duration_ms=%u\n",button.sequence,button.duration_ms);
        else unavailable("button.duration_snapshot");
        close(fd);
    }
    fd = open("/dev/pwm", O_RDONLY | O_NONBLOCK);
    if (fd < 0) unavailable("backlight.open");
    else {
        /* This driver returns the level directly, not through an output pointer. */
        level = ioctl(fd,IOR_BACKLIGHT_CURRENT,0);
        if (level >= 0) printf("BACKLIGHT level=%d\n",level);
        else unavailable("backlight.current");
        close(fd);
    }
    fd = open("/dev/battery", O_RDONLY | O_NONBLOCK);
    if (fd < 0) unavailable("battery.open");
    else {
        memset(&battery,0,sizeof battery);
        if (ioctl(fd,IOR_BATTERY_STATUS,&battery) == 0)
            printf("BATTERY voltage_mv=%u charge_state=%u (driver charge-current field is not a measurement)\n",
                   battery.u16BatteryVoltage,battery.u8ChargeStatus);
        else unavailable("battery.status");
        close(fd);
    }
    fd = open("/dev/rtc", O_RDONLY | O_NONBLOCK);
    if (fd < 0) unavailable("rtc.open");
    else {
        if (ioctl(fd,RTC_RD_TIME,&rtc) == 0)
            printf("RTC %04d-%02d-%02dT%02d:%02d:%02d\n",rtc.tm_year+1900,rtc.tm_mon+1,rtc.tm_mday,rtc.tm_hour,rtc.tm_min,rtc.tm_sec);
        else unavailable("rtc.read_time");
        close(fd);
    }
    printf("INFO probe is capability inventory; unavailable capabilities do not imply test success\n");
    return 0;
}

static int selftest(void)
{
    char dir[]="/tmp/fracture-selftest-XXXXXX";
    char first[160],second[160],buf[64];
    const char payload[]="FractureOS-v3 local fixture\n";
    int fd, pair[2], status=0;
    pid_t child;
    void *mem;
    struct pollfd p;
    struct timespec before,after;
    int ok;
    FILE *memory;
    result("abi.arm32", sizeof(void*)==4 && sizeof(long)==4);
    result("identity.root", getuid()==0);
    fd=open("/dev/urandom",O_RDONLY|O_NONBLOCK);
    result("urandom.read_available_NOT_entropy_validation",fd>=0 && read(fd,buf,32)==32);
    if(fd>=0)close(fd);
    ok=syscall(__NR_clock_gettime,1,&before)==0;
    usleep(20000);
    ok=ok && syscall(__NR_clock_gettime,1,&after)==0;
    result("clock.monotonic_advances",ok && (after.tv_sec>before.tv_sec ||
        (after.tv_sec==before.tv_sec && after.tv_nsec>before.tv_nsec)));
    mem=mmap(NULL,4096,PROT_READ|PROT_WRITE,MAP_PRIVATE|MAP_ANONYMOUS,-1,0);
    result("memory.anonymous_mmap",mem!=MAP_FAILED);
    if(mem!=MAP_FAILED){memset(mem,0xa5,4096);munmap(mem,4096);}
    if(socketpair(AF_UNIX,SOCK_STREAM,0,pair)==0){
        child=fork();
        if(child==0){close(pair[0]);ok=write(pair[1],"OK",2)==2;close(pair[1]);_exit(ok?0:3);}
        close(pair[1]);
        p.fd=pair[0];p.events=POLLIN;p.revents=0;
        ok=child>0 && poll(&p,1,1000)>0 && read(pair[0],buf,2)==2 && !memcmp(buf,"OK",2);
        if(child>0)ok=(waitpid(child,&status,0)==child && WIFEXITED(status) && WEXITSTATUS(status)==0) && ok;
        result("ipc.fork_socketpair_poll_wait",ok);close(pair[0]);
    }else result("ipc.socketpair",0);
    if(!mkdtemp(dir)){result("storage.tmp_directory",0);return 1;}
    snprintf(first,sizeof first,"%s/pending",dir);
    snprintf(second,sizeof second,"%s/committed",dir);
    fd=open(first,O_WRONLY|O_CREAT|O_EXCL,0600);
    ok=fd>=0 && write(fd,payload,sizeof payload)==sizeof payload && fsync(fd)==0;
    if(fd>=0)close(fd);
    ok=ok && rename(first,second)==0;
    fd=open(second,O_RDONLY);
    ok=ok && fd>=0 && read(fd,buf,sizeof payload)==sizeof payload && !memcmp(buf,payload,sizeof payload);
    if(fd>=0)close(fd);
    result("storage.tmp_write_fsync_rename_read",ok);
    unlink(first);unlink(second);rmdir(dir);
    memory=fopen("/proc/self/status","r");
    if(memory){
        while(fgets(buf,sizeof buf,memory))
            if(!strncmp(buf,"Vm",2))printf("MEMORY %s",buf);
        fclose(memory);
    }
    printf("SUMMARY failures=%d (temporary RAM filesystem only; not FAT power-loss validation)\n",failures);
    return failures?1:0;
}

int main(int argc,char **argv)
{
    unsigned long offset,count;
    char *end;
    alarm(30); /* Bound failed driver/filesystem probes without a timeout applet. */
    if(argc==2 && !strcmp(argv[1],"--probe"))return probe();
    if(argc==2 && !strcmp(argv[1],"--selftest"))return selftest();
    if(argc==3 && !strcmp(argv[1],"--hex"))return export_hex(argv[2],0,8*1024*1024);
    if(argc==5 && !strcmp(argv[1],"--hex")){
        errno=0;offset=strtoul(argv[3],&end,10);
        if(errno || !argv[3][0] || *end || offset>8*1024*1024)return 2;
        errno=0;count=strtoul(argv[4],&end,10);
        if(errno || !argv[4][0] || *end || count>65536)return 2;
        return export_hex(argv[2],offset,count);
    }
    fprintf(stderr,"Usage: %s --probe | --selftest | --hex REGULAR_FILE [OFFSET COUNT]\n",argv[0]);
    return 2;
}
