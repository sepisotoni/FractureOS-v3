/* FractureOS v3: small framebuffer clock and switch-operated launcher.
 * Original v3 code; Linux/Barcelona ioctl ABI supplied by the OpenTom SDK.
 */
#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <errno.h>
#include <stdint.h>
#include <signal.h>
#include <unistd.h>
#include <fcntl.h>
#include <time.h>
#include <poll.h>
#include <sys/file.h>
#include <sys/mman.h>
#include <sys/ioctl.h>
#include <sys/socket.h>
#include <sys/un.h>
#include <sys/stat.h>
#include <sys/time.h>
#include <sys/reboot.h>
#include <linux/fb.h>
#include <linux/input.h>
#ifdef __arm__
#include <asm/unistd.h>
#include <barcelona/Barc_Gpio.h>
#include <barcelona/Barc_pwm.h>
#ifndef __NR_clock_gettime
#define __NR_clock_gettime (__NR_SYSCALL_BASE+263)
#endif
#endif
#ifndef FRACTURE_VERSION
#define FRACTURE_VERSION "0.1.8"
#endif
#define W 320
#define H 240
#define SOCK "/tmp/fracture-v3.sock"
typedef struct {int offset,brightness,profile,theme,alarm_hour,alarm_minute,last_alarm_day;char face[32],theme_name[32];int hold_indicator;} Settings;
static Settings cfg={0,30,0,0,-1,0,-1,"nunito","teal",1};
static const char *config_path="/mnt/sdcard/fracture-v3/settings.cfg";
static const char *assets_path="/mnt/sdcard/fracture-v3/faces";
static const char *profiles[]={"minimal","desk","bedside"};
static const char *themes[]={"teal","amber","blue"};
static uint16_t pixels[W*H];
static unsigned char *mapped;
static size_t mapped_size;
static int fb=-1,input=-1,gpio=-1,light=-1,listener=-1,lockfd=-1,old_light=-1;
static struct fb_fix_screeninfo fix;
static struct fb_var_screeninfo var;
static volatile sig_atomic_t running=1;
static int menu=0,selected=0,alert=0,touch_down=0,touch_held=0;
static int cpu=-1,ram=-1,disk_usage=-1,net_down=-1,net_up=-1,dirty=1,frame_count=0;
static double timer_end=0,watch_start=0,watch_elapsed=0,stats_at=0,touch_at=0,tap_due=0,power_due=0;
static unsigned int last_button=0;
static time_t wall_override=0;


/* Five-column fallback glyphs, available if the UI atlas is missing. */
static const unsigned char font[][5]={
 {0x3e,0x51,0x49,0x45,0x3e},{0,0x42,0x7f,0x40,0},{0x42,0x61,0x51,0x49,0x46},{0x21,0x41,0x45,0x4b,0x31},{0x18,0x14,0x12,0x7f,0x10},
 {0x27,0x45,0x45,0x45,0x39},{0x3c,0x4a,0x49,0x49,0x30},{1,0x71,9,5,3},{0x36,0x49,0x49,0x49,0x36},{6,0x49,0x49,0x29,0x1e},
 {0x7e,0x11,0x11,0x11,0x7e},{0x7f,0x49,0x49,0x49,0x36},{0x3e,0x41,0x41,0x41,0x22},{0x7f,0x41,0x41,0x22,0x1c},{0x7f,0x49,0x49,0x49,0x41},
 {0x7f,9,9,9,1},{0x3e,0x41,0x49,0x49,0x7a},{0x7f,8,8,8,0x7f},{0,0x41,0x7f,0x41,0},{0x20,0x40,0x41,0x3f,1},
 {0x7f,8,0x14,0x22,0x41},{0x7f,0x40,0x40,0x40,0x40},{0x7f,2,0xc,2,0x7f},{0x7f,4,8,0x10,0x7f},{0x3e,0x41,0x41,0x41,0x3e},
 {0x7f,9,9,9,6},{0x3e,0x41,0x51,0x21,0x5e},{0x7f,9,0x19,0x29,0x46},{0x46,0x49,0x49,0x49,0x31},{1,1,0x7f,1,1},
 {0x3f,0x40,0x40,0x40,0x3f},{0x1f,0x20,0x40,0x20,0x1f},{0x3f,0x40,0x38,0x40,0x3f},{0x63,0x14,8,0x14,0x63},{7,8,0x70,8,7},{0x61,0x51,0x49,0x45,0x43}};
static double mono(void){struct timespec t;
#ifdef __arm__
 if(syscall(__NR_clock_gettime,1,&t))return 0;
#else
 if(clock_gettime(CLOCK_MONOTONIC,&t))return 0;
#endif
 return t.tv_sec+t.tv_nsec/1e9;}
static uint16_t rgb(int r,int g,int b){return (uint16_t)(((r>>3)<<11)|((g>>2)<<5)|(b>>3));}
static void rect(int x,int y,int width,int height,uint16_t color){int i,j;
 for(j=y;j<y+height && j<H;j++)for(i=x;i<x+width && i<W;i++)if(i>=0 && j>=0)pixels[j*W+i]=color;}

#include "ui.h"
static void text(int x,int y,const char *s,int scale,uint16_t color){int i,j,k;
 if(ui_font){ui_text(x,y,s,scale,color);return;}
 for(;*s;s++,x+=6*scale){unsigned char c=(unsigned char)*s;const unsigned char *g=NULL;
 if(c>='a'&&c<='z')c-=32;
 if(c>='0'&&c<='9')g=font[c-'0'];else if(c>='A'&&c<='Z')g=font[c-'A'+10];
 if(g){for(i=0;i<5;i++)for(j=0;j<7;j++)if(g[i]&(1<<j))rect(x+i*scale,y+j*scale,scale,scale,color);}
 else if(c==':'){rect(x+2*scale,y+2*scale,scale,scale,color);rect(x+2*scale,y+5*scale,scale,scale,color);}
 else if(c=='-'||c=='+'){rect(x,y+3*scale,5*scale,scale,color);if(c=='+')rect(x+2*scale,y+scale,scale,5*scale,color);}
 else if(c=='/'){for(k=0;k<5;k++)rect(x+k*scale,y+(5-k)*scale,scale,scale,color);}
 else if(c=='.')rect(x+scale,y+6*scale,scale,scale,color);
 else if(c=='%'){rect(x,y,scale,scale,color);rect(x+4*scale,y+6*scale,scale,scale,color);for(k=0;k<5;k++)rect(x+k*scale,y+(5-k)*scale,scale,scale,color);}
 }}
static void center(int y,const char *s,int scale,uint16_t color){text(ui_font?(W-ui_width(s,scale))/2:(W-(int)strlen(s)*6*scale+scale)/2,y,s,scale,color);}
static void text_fit(int x,int y,const char *s,int width,int scale,uint16_t color){char out[160];size_t n;snprintf(out,sizeof out,"%s",s);if(ui_font){while(scale>1&&ui_width(out,scale)>width)scale--;n=strlen(out);while(n&&ui_width(out,scale)>width)out[--n]=0;if(n<strlen(s)&&n>2){out[n-1]='.';out[n-2]='.';}}else{n=strlen(out);while(n&&(int)n*6*scale>width)out[--n]=0;}text(x,y,out,scale,color);}

#include "themes.h"
#include "faces.h"
static void localtime_now(struct tm *out){time_t t=(wall_override?wall_override:time(NULL))+cfg.offset*60;struct tm *v=gmtime(&t);if(v)*out=*v;else memset(out,0,sizeof *out);}
static int save(void){char tmp[512];FILE *f;int ok;
 if(strlen(config_path)>470)return -1;snprintf(tmp,sizeof tmp,"%s.new",config_path);
 f=fopen(tmp,"w");if(!f)return -1;
 fprintf(f,"version=1\noffset=%d\nbrightness=%d\nprofile=%d\ntheme=%d\nalarm_hour=%d\nalarm_minute=%d\nlast_alarm_day=%d\nface=%s\ntheme_name=%s\nhold_indicator=%d\n",cfg.offset,cfg.brightness,cfg.profile,cfg.theme,cfg.alarm_hour,cfg.alarm_minute,cfg.last_alarm_day,cfg.face,cfg.theme_name,cfg.hold_indicator);
 ok=fflush(f)==0 && fsync(fileno(f))==0;if(fclose(f))ok=0;
 if(!ok || rename(tmp,config_path)){unlink(tmp);return -1;}return 0;}
static void load(void){char line[128],key[40];int n;FILE *f=fopen(config_path,"r");if(!f)return;
 while(fgets(line,sizeof line,f)){if(!strncmp(line,"theme_name=",11)){char id[32];if(sscanf(line+11,"%31s",id)==1&&theme_id(id))strcpy(cfg.theme_name,id);continue;}if(!strncmp(line,"face=",5)){char id[32];if(sscanf(line+5,"%31s",id)==1&&identifier(id))strcpy(cfg.face,id);continue;}if(sscanf(line,"%39[^=]=%d",key,&n)==2){
 if(!strcmp(key,"offset")&&n>=-720&&n<=840)cfg.offset=n;
 else if(!strcmp(key,"brightness")&&n>=1&&n<=100)cfg.brightness=n;
 else if(!strcmp(key,"profile")&&n>=0&&n<3)cfg.profile=n;
 else if(!strcmp(key,"theme")&&n>=0&&n<3){cfg.theme=n;strcpy(cfg.theme_name,themes[n]);}
 else if(!strcmp(key,"alarm_hour")&&n>=-1&&n<24)cfg.alarm_hour=n;
 else if(!strcmp(key,"alarm_minute")&&n>=0&&n<60)cfg.alarm_minute=n;
 else if(!strcmp(key,"hold_indicator")&&n>=0&&n<=1)cfg.hold_indicator=n;
 else if(!strcmp(key,"last_alarm_day"))cfg.last_alarm_day=n;}}
 fclose(f);}
static void brightness(void){
#ifdef __arm__
 if(light>=0)ioctl(light,IOW_BACKLIGHT_UPDATE,cfg.profile==2?5:cfg.brightness);
#endif
}
#include "media.h"
#include "navigation.h"
#include "power.h"
static void render(void){char s[64];int rows,height;struct tm t;uint16_t bg=theme_color(theme.background),fg=theme_color(theme.foreground),muted=theme_color(theme.muted),accent=theme_color(theme.accent);
 rect(0,0,W,H,bg);localtime_now(&t);
 if(alert){center(50,alert==2?"DAILY ALARM":"TIME IS UP",3,accent);center(130,"TAP TO DISMISS",2,fg);center(200,"SILENT VISUAL ALERT",1,muted);power_draw();return;}
 if(menu){navigation_draw();power_draw();return;}
 rows=status_rows();height=H-(rows?rows*27+12:0);
 if(face_ready&&t.tm_year>=120){if(height==H)face_draw(&t);else face_draw_height(&t,height);}
 else if(t.tm_year<120){center(65,"SET TIME",4,accent);center(120,"CONNECT HUB OR USE CLI",1,fg);}
 else{strftime(s,sizeof s,"%H:%M",&t);center(60,s,5,fg);strftime(s,sizeof s,"%a %d %b",&t);center(125,s,2,muted);}
 if(rows)status_draw(height);power_draw();}
static int ppm(const char *path){FILE *f=fopen(path,"wb");int i;if(!f)return -1;fprintf(f,"P6\n320 240\n255\n");for(i=0;i<W*H;i++){unsigned char v[3];v[0]=((pixels[i]>>11)&31)*255/31;v[1]=((pixels[i]>>5)&63)*255/63;v[2]=(pixels[i]&31)*255/31;fwrite(v,1,3,f);}return fclose(f);}
static void present(void){int y;if(!mapped)return;
 /* Small full-frame copies on actual redraws avoid stale dirty-row assumptions
  * when the legacy framebuffer is touched by a diagnostic/console client.
  * No idle frame loop, and no second 150 KB shadow framebuffer is needed. */
 if(fix.line_length==W*2&&var.xoffset==0)memcpy(mapped+var.yoffset*fix.line_length,pixels,sizeof pixels);
 else for(y=0;y<H;y++)memcpy(mapped+(y+var.yoffset)*fix.line_length+var.xoffset*2,pixels+y*W,W*2);frame_count++;}

static int integer(const char *s,int lo,int hi,int *value){char *end;long n;errno=0;n=strtol(s,&end,10);if(errno||end==s||*end||n<lo||n>hi)return 0;*value=(int)n;return 1;}
static void command(char *line,char *reply,size_t cap){char *name,*a,*b,*extra;int n,m,i;Settings old=cfg;struct tm t;
 name=strtok(line," \r\n");a=strtok(NULL," \r\n");b=strtok(NULL," \r\n");extra=strtok(NULL," \r\n");snprintf(reply,cap,"ERR invalid command\n");if(!name||extra)return;
 if(media_command(name,a,b)){dirty=1;snprintf(reply,cap,"OK\n");return;}
 if(!strcmp(name,"status")&&!a){localtime_now(&t);snprintf(reply,cap,"{\"version\":\"%s\",\"profile\":\"%s\",\"theme\":\"%s\",\"face\":\"%s\",\"face_loaded\":%s,\"brightness\":%d,\"offset\":%d,\"alarm_hour\":%d,\"alarm_minute\":%d,\"timer_seconds\":%d,\"menu\":%d,\"selection\":%d,\"frames\":%d,\"time_valid\":%s,\"hold_live\":%s,\"power_down\":%d,\"stopwatch_seconds\":%d,\"stopwatch_running\":%s,\"media_view\":%s,\"media_art_loaded\":%d,\"media_seq\":%u,\"media_action\":\"%s\",\"media_arg\":%d,\"media_pending\":%s}\n",FRACTURE_VERSION,profiles[cfg.profile],cfg.theme_name,cfg.face,face_ready?"true":"false",cfg.brightness,cfg.offset,cfg.alarm_hour,cfg.alarm_minute,timer_end>mono()?(int)(timer_end-mono()):0,menu,selected,frame_count,t.tm_year>=120?"true":"false",power_regs?"true":"false",power_down,(int)(watch_elapsed+(watch_start?mono()-watch_start:0)),watch_start?"true":"false",menu==PAGE_PLAYER||menu==PAGE_MEDIA_MORE||(menu==PAGE_EDIT&&edit_parent==PAGE_PLAYER)?"true":"false",media_art_loaded,media_sequence,media_action,media_argument,media_sequence!=media_ack&&mono()-media_action_at<10?"true":"false");return;}
 if(!strcmp(name,"capture")&&!a){render();present();snprintf(reply,cap,ppm("/tmp/fracture-v3-screen.ppm")?"ERR capture\n":"OK\n");return;}
 if(!strcmp(name,"theme-get")&&!a){theme_json(reply,cap);return;}
 if(!strcmp(name,"theme-reset")&&!a){char path[640];Theme next=theme;theme_path(path,sizeof path,0);if(!theme_read(path,&next))return;theme_path(path,sizeof path,1);if(unlink(path)&&errno!=ENOENT)return;theme=next;dirty=1;snprintf(reply,cap,"OK\n");return;}
 if(!strcmp(name,"face-get")&&!a){if(face_ready)face_json(reply,cap);return;}
 if(!strcmp(name,"face-reset")&&!a){char path[640];face_path(path,sizeof path,1);if(unlink(path)&&errno!=ENOENT)return;if(!face_load())return;dirty=1;snprintf(reply,cap,"OK\n");return;}
 if(!a)return;
 if(!strcmp(name,"hold-indicator")&&!b&&integer(a,0,1,&n)){cfg.hold_indicator=n;if(save()){cfg=old;return;}dirty=1;snprintf(reply,cap,"OK\n");return;}
 if(!strcmp(name,"stopwatch")&&!b){if(!strcmp(a,"toggle"))watch_toggle();else if(!strcmp(a,"reset"))watch_start=watch_elapsed=watch_lap=0;else if(!strcmp(a,"lap"))watch_lap=watch_elapsed+(watch_start?mono()-watch_start:0);else return;dirty=1;snprintf(reply,cap,"OK\n");return;}

 if(!strcmp(name,"theme-set")&&b){Theme next=theme;if(!theme_field(&next,a,b))return;if(!theme_save(&next)){snprintf(reply,cap,"ERR theme persistence\n");return;}theme=next;dirty=1;snprintf(reply,cap,"OK\n");return;}
 if(!strcmp(name,"theme-save")&&!b){char path[640];FILE *exists;if(!theme_id(a))return;strcpy(cfg.theme_name,a);theme_path(path,sizeof path,1);exists=fopen(path,"r");if(exists){fclose(exists);cfg=old;snprintf(reply,cap,"ERR theme name already exists\n");return;}if(!theme_save(&theme)||save()){cfg=old;return;}dirty=1;snprintf(reply,cap,"OK\n");return;}
 if(!strcmp(name,"face")&&!b){if(strlen(a)>=sizeof cfg.face||!identifier(a))return;strcpy(cfg.face,a);if(!face_load()){cfg=old;return;}if(save()){cfg=old;face_load();return;}dirty=1;snprintf(reply,cap,"OK\n");return;}
 if(!strcmp(name,"face-set")&&b&&face_ready){Face next=face;unsigned char *data=NULL;if(!face_field(&next,a,b))return;
 if(strcmp(next.atlas,face.atlas)&&!face_atlas(&next,&data))return;
 if(!face_save(&next)){free(data);snprintf(reply,cap,"ERR face persistence\n");return;}if(data){free(face_pixels);face_pixels=data;}face=next;dirty=1;snprintf(reply,cap,"OK\n");return;}

 if(!strcmp(name,"profile")&&!b){for(i=0;i<3;i++)if(!strcmp(a,profiles[i])){cfg.profile=i;break;}if(i==3)return;}
 else if(!strcmp(name,"theme")&&!b){if(!theme_id(a))return;strcpy(cfg.theme_name,a);if(!theme_load()){cfg=old;return;}}
 else if(!strcmp(name,"brightness")&&!b&&integer(a,1,100,&n))cfg.brightness=n;
 else if(!strcmp(name,"offset")&&!b&&integer(a,-720,840,&n))cfg.offset=n;
 else if(!strcmp(name,"timer")&&!b&&integer(a,0,86400,&n)){timer_end=n?mono()+n:0;alert=0;}
 else if(!strcmp(name,"alarm")&&!b&&!strcmp(a,"off"))cfg.alarm_hour=-1;
 else if(!strcmp(name,"alarm")&&b&&integer(a,0,23,&n)&&integer(b,0,59,&m)){cfg.alarm_hour=n;cfg.alarm_minute=m;}
 else if(!strcmp(name,"stats-extra")&&a&&b&&integer(a,-1,100,&n)){char extra;int down,up;if(sscanf(b,"%d,%d%c",&down,&up,&extra)!=2||down< -1||up< -1)return;disk_usage=n;net_down=down;net_up=up;}
 else if(!strcmp(name,"stats")&&b&&integer(a,0,100,&n)&&integer(b,0,100,&m)){cpu=n;ram=m;stats_at=mono();}
 else if(!strcmp(name,"event")&&!b){if(strcmp(a,"tap")&&strcmp(a,"double")&&strcmp(a,"hold")&&strcmp(a,"power")&&strcmp(a,"long")&&strcmp(a,"home")&&strcmp(a,"back"))return;event(a);}
 else if(!strcmp(name,"time")&&!b&&integer(a,1700000000,2147483647,&n)){struct timeval tv;tv.tv_sec=n;tv.tv_usec=0;if(settimeofday(&tv,NULL)){snprintf(reply,cap,"ERR time unavailable\n");return;}}
 else return;
 if(memcmp(&old,&cfg,sizeof cfg)&&save()){cfg=old;theme_load();snprintf(reply,cap,"ERR settings persistence\n");return;}brightness();dirty=1;snprintf(reply,cap,"OK\n");}
static void stop(int sig){(void)sig;running=0;}
static int control(int argc,char **argv){int s,i;struct sockaddr_un addr;char line[256],reply[2048];size_t used=0;ssize_t n;
 if(argc<1)return 2;line[0]=0;for(i=0;i<argc;i++){if(strlen(argv[i])+used+2>=sizeof line)return 2;used+=(size_t)snprintf(line+used,sizeof line-used,"%s%s",i?" ":"",argv[i]);}strcat(line,"\n");
 s=socket(AF_UNIX,SOCK_STREAM,0);if(s<0)return 1;memset(&addr,0,sizeof addr);addr.sun_family=AF_UNIX;strcpy(addr.sun_path,SOCK);if(connect(s,(struct sockaddr*)&addr,sizeof addr)){perror("connect");close(s);return 1;}
 if(write(s,line,strlen(line))!=(ssize_t)strlen(line)){close(s);return 1;}used=0;while(used+1<sizeof reply&&(n=read(s,reply+used,sizeof reply-used-1))>0)used+=n;reply[used]=0;close(s);fputs(reply,stdout);return used&&strncmp(reply,"ERR",3)?0:1;}
static int framebuffer(void){size_t end;fb=open("/dev/fb",O_RDWR);if(fb<0)return -1;if(ioctl(fb,FBIOGET_FSCREENINFO,&fix)||ioctl(fb,FBIOGET_VSCREENINFO,&var))return -1;
 end=(var.yoffset+H-1)*fix.line_length+(var.xoffset+W)*2;if(var.xres!=W||var.yres!=H||var.bits_per_pixel!=16||var.red.offset!=11||var.green.offset!=5||var.blue.offset!=0||end>fix.smem_len)return -1;
 mapped_size=fix.smem_len;mapped=mmap(NULL,mapped_size,PROT_READ|PROT_WRITE,MAP_SHARED,fb,0);if(mapped==MAP_FAILED){mapped=NULL;return -1;}return 0;}
static void inputs(double now){struct input_event e;ssize_t n;
 if(input>=0)while((n=read(input,&e,sizeof e))==(ssize_t)sizeof e){int down=-1;if(e.type==EV_KEY&&e.code==BTN_TOUCH)down=e.value!=0;if(e.type==EV_ABS&&e.code==ABS_PRESSURE)down=e.value>0;
 if(down==1&&!touch_down){touch_down=1;touch_at=now;touch_held=0;}else if(down==0&&touch_down){touch_down=0;if(!touch_held&&now-touch_at>=0.06){if(tap_due&&now<tap_due){tap_due=0;event("double");}else tap_due=now+0.32;}}}
 if(touch_down&&!touch_held&&now-touch_at>=0.7){touch_held=1;tap_due=0;event("hold");}
 if(tap_due&&now>=tap_due){tap_due=0;event("tap");}
 power_poll(now);
 if(power_due&&now>=power_due){power_due=0;event("power");}}
static int daemon_run(int seconds){struct sockaddr_un addr;struct pollfd p[2];double start=mono(),last_tick=-1;int count,client;char line[256],reply[2048];FILE *pid;struct tm t;
 lockfd=open("/tmp/fracture-v3.lock",O_CREAT|O_RDWR,0600);if(lockfd<0||flock(lockfd,LOCK_EX|LOCK_NB)){fprintf(stderr,"Already running\n");return 1;}
 if(framebuffer()){perror("framebuffer");return 1;}listener=socket(AF_UNIX,SOCK_STREAM,0);if(listener<0)return 1;memset(&addr,0,sizeof addr);addr.sun_family=AF_UNIX;strcpy(addr.sun_path,SOCK);unlink(SOCK);umask(077);
 if(bind(listener,(struct sockaddr*)&addr,sizeof addr)||listen(listener,4)){perror("control socket");return 1;}
 input=open("/dev/input/event0",O_RDONLY|O_NONBLOCK);
#ifdef __arm__
 gpio=open("/dev/hwstatus",O_RDONLY|O_NONBLOCK);if(gpio>=0){GPIO_BUTTON_EVENT b;if(!ioctl(gpio,IOR_BUTTON_EVENT,&b))last_button=b.sequence;}
 light=open("/dev/pwm",O_RDONLY|O_NONBLOCK);if(light>=0)old_light=ioctl(light,IOR_BACKLIGHT_CURRENT,0);
#endif
 power_open();brightness();signal(SIGTERM,stop);signal(SIGINT,stop);signal(SIGPIPE,SIG_IGN);pid=fopen("/tmp/fracture-v3.pid","w");if(pid){fprintf(pid,"%ld\n",(long)getpid());fclose(pid);}
 fprintf(stderr,"FractureOS %s running\n",FRACTURE_VERSION);
 while(running){double now=mono();if(seconds>0&&now-start>=seconds)break;inputs(now);if(notice_until&&now>=notice_until){notice_until=0;notification[0]=0;dirty=1;}if(media_at&&now-media_at>=20){media_at=0;media_release_art();dirty=1;}if(menu_anim_start){dirty=1;if(now-menu_anim_start>=0.18)menu_anim_start=0;}
 if(timer_end&&now>=timer_end){timer_end=0;alert=1;dirty=1;}
 localtime_now(&t);if(t.tm_year>=120&&cfg.alarm_hour==t.tm_hour&&cfg.alarm_minute==t.tm_min&&cfg.last_alarm_day!=(t.tm_year*366+t.tm_yday)){cfg.last_alarm_day=t.tm_year*366+t.tm_yday;if(!save()){alert=2;dirty=1;}}
 {double tick=(timer_end||watch_start||cfg.profile==1||menu==PAGE_PLAYER||(face_ready&&!menu&&!alert&&(face.show_seconds||(face.colon&&face.blink&&!(face.hour_only&&t.tm_min==0)))))?(double)time(NULL):(double)(time(NULL)/60);if(tick!=last_tick){dirty=1;last_tick=tick;}}
 if(dirty){render();present();dirty=0;}
 p[0].fd=listener;p[0].events=POLLIN;p[0].revents=0;p[1].fd=input;p[1].events=POLLIN;p[1].revents=0;count=poll(p,2,50);if(count>0&&(p[0].revents&POLLIN)){client=accept(listener,NULL,NULL);if(client>=0){struct timeval timeout={1,0};size_t used=0;ssize_t got;setsockopt(client,SOL_SOCKET,SO_RCVTIMEO,&timeout,sizeof timeout);setsockopt(client,SOL_SOCKET,SO_SNDTIMEO,&timeout,sizeof timeout);
 while(used+1<sizeof line){got=read(client,line+used,1);if(got!=1)break;if(line[used++]=='\n')break;}line[used]=0;if(used&&line[used-1]=='\n'){command(line,reply,sizeof reply);if(write(client,reply,strlen(reply))<0)dirty=1;}close(client);}}}
#ifdef __arm__
 if(light>=0&&old_light>=0)ioctl(light,IOW_BACKLIGHT_UPDATE,old_light);
#endif
 if(power_regs)munmap((void*)power_regs,4096);media_release_art();if(mapped)munmap(mapped,mapped_size);if(fb>=0)close(fb);if(input>=0)close(input);if(gpio>=0)close(gpio);if(light>=0)close(light);close(listener);unlink(SOCK);unlink("/tmp/fracture-v3.pid");close(lockfd);return 0;}
int main(int argc,char **argv){int i,seconds=0;char asset_directory[640];const char *preview=NULL,*snapshot=NULL,*slash=strrchr(argv[0],'/');
 if(slash){snprintf(asset_directory,sizeof asset_directory,"%.*s/faces",(int)(slash-argv[0]),argv[0]);assets_path=asset_directory;}
 if(argc>1&&!strcmp(argv[1],"ctl")){alarm(5);return control(argc-2,argv+2);}
 for(i=1;i<argc;i++){if(!strcmp(argv[i],"--version")){puts(FRACTURE_VERSION);return 0;}else if(!strcmp(argv[i],"--config")&&i+1<argc)config_path=argv[++i];else if(!strcmp(argv[i],"--assets")&&i+1<argc)assets_path=argv[++i];else if(!strcmp(argv[i],"--seconds")&&i+1<argc)seconds=atoi(argv[++i]);else if(!strcmp(argv[i],"--preview")&&i+1<argc)preview=argv[++i];else if(!strcmp(argv[i],"--snapshot")&&i+1<argc)snapshot=argv[++i];else if(!strcmp(argv[i],"--at")&&i+1<argc)wall_override=(time_t)atol(argv[++i]);else{fprintf(stderr,"Unknown argument\n");return 2;}}
 load();theme_load();face_load();ui_font_load();if(preview){render();return ppm(preview)?1:0;}if(snapshot){int y;if(framebuffer())return 1;for(y=0;y<H;y++)memcpy(pixels+y*W,mapped+(y+var.yoffset)*fix.line_length+var.xoffset*2,W*2);return ppm(snapshot)?1:0;}return daemon_run(seconds);}
