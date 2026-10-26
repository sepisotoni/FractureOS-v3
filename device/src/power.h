/* Casablanca/S3C2412 GPF0 is active low. Read-only mapping: no register writes.
 * SDK: tomtomgo-iocasablanca-*.h, regs-gpio.h; gate by exact model and CPU. */
static const volatile unsigned int *power_regs;
static int power_down,power_armed,power_second;
static double power_started,power_ignore_until;
static void power_open(void){
#ifdef __arm__
 FILE *f;int model=0,cputype=0,fd;void *p;f=fopen("/proc/barcelona/modelid","r");if(f){if(fscanf(f,"%d",&model)!=1)model=0;fclose(f);}f=fopen("/proc/barcelona/cputype","r");if(f){if(fscanf(f,"%d",&cputype)!=1)cputype=0;fclose(f);}if(model!=19||cputype!=4)return;
 fd=open("/dev/mem",O_RDONLY|O_SYNC);if(fd<0)return;p=mmap(NULL,4096,PROT_READ,MAP_SHARED,fd,0x56000000);close(fd);if(p==MAP_FAILED)return;power_regs=(const volatile unsigned int*)p;
 /* GPF0 must be input or EINT, never an output. Ignore any hold across startup. */
 if((power_regs[0x50/4]&3)!=0&&(power_regs[0x50/4]&3)!=2){munmap(p,4096);power_regs=NULL;return;}power_armed=(power_regs[0x54/4]&1)!=0;
#endif
}
static void power_action(int action){
#ifdef __arm__
 int fd,ok;if(save()){info_text="Settings could not be saved";page_open(PAGE_INFO);return;}sync();
 if(action==1){fd=open("/sys/power/state",O_WRONLY);if(fd<0){info_text="Suspend is unavailable";page_open(PAGE_INFO);return;}
 if(light>=0)ioctl(light,IOW_BACKLIGHT_UPDATE,0);ok=write(fd,"mem\n",4)==4;close(fd);brightness();power_ignore_until=mono()+1;power_down=0;power_armed=0;tap_due=power_due=0;touch_down=touch_held=0;page_open(PAGE_CLOCK);if(!ok){info_text="Suspend failed";page_open(PAGE_INFO);}return;}
 if(action==2)reboot(RB_AUTOBOOT);else if(action==3)reboot(RB_POWER_OFF);
 info_text="Power request failed";page_open(PAGE_INFO);
#else
 (void)action;info_text="Power actions require the TomTom";page_open(PAGE_INFO);
#endif
}
static void power_release(unsigned int ms){int second=power_second;power_second=0;if(ms<60)return;if(ms>=7000){power_action(1);return;}if(ms>=4500)return;if(ms>=3000){power_due=0;event("home");return;}if(ms>=1500){power_due=0;event("back");return;}
 if(second||(power_due&&mono()<power_due)){power_due=0;event("home");}else power_due=mono()+0.32;}
static void power_poll(double now){if(power_regs){int down=(power_regs[0x54/4]&1)==0;if(now<power_ignore_until)return;if(!power_armed){if(!down)power_armed=1;return;}if(down&&!power_down){power_down=1;power_started=now;power_second=power_due&&now<power_due;tap_due=power_due=0;}else if(!down&&power_down){power_down=0;dirty=1;power_release((unsigned int)((now-power_started)*1000));}if(power_down)dirty=1;return;}
#ifdef __arm__
 if(gpio>=0){GPIO_BUTTON_EVENT b;if(!ioctl(gpio,IOR_BUTTON_EVENT,&b)&&b.sequence!=last_button){last_button=b.sequence;if(now>=power_ignore_until&&b.duration_ms!=0xffffffffUL)power_release(b.duration_ms);}}
#endif
}
static void power_draw(void){double elapsed;uint16_t c;const char *label;if(!power_down||!cfg.hold_indicator)return;elapsed=mono()-power_started;if(elapsed<0.15)return;if(elapsed>=4.5&&elapsed<7)return;c=elapsed<1.5?rgb(48,209,88):elapsed<3?rgb(10,132,255):rgb(191,90,242);label=elapsed<1.5?"Select":elapsed<3?"Back":elapsed<4.5?"Home":"Release to sleep";rounded(180,5,132,28,14,rgb(28,28,30));rounded(190,14,10,10,5,c);text_fit(207,10,label,99,1,rgb(245,245,247));}
