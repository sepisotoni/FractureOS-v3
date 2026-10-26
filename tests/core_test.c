#define main fracture_program_main
#include "../device/src/fracture.c"
#undef main
#include <assert.h>
int main(void){char path[]="/tmp/fracture-core-test-XXXXXX",line[128],reply[2048];int fd=mkstemp(path);assert(fd>=0);close(fd);config_path=path;
 strcpy(line,"profile bedside");command(line,reply,sizeof reply);assert(!strcmp(reply,"OK\n"));assert(cfg.profile==2);
 strcpy(line,"brightness 101");command(line,reply,sizeof reply);assert(!strncmp(reply,"ERR",3));assert(cfg.brightness==30);
 strcpy(line,"offset -720");command(line,reply,sizeof reply);assert(cfg.offset==-720);
 cfg.offset=0;load();assert(cfg.offset==-720);
 strcpy(line,"timer 3");command(line,reply,sizeof reply);assert(timer_end>mono());
 strcpy(line,"timer 0");command(line,reply,sizeof reply);assert(timer_end==0);
 strcpy(line,"event power");command(line,reply,sizeof reply);assert(menu==1);
 strcpy(line,"event tap");command(line,reply,sizeof reply);assert(selected==1);
 strcpy(line,"event power");command(line,reply,sizeof reply);assert(menu==0&&cfg.profile==1);
 strcpy(line,"event long");command(line,reply,sizeof reply);assert(menu==1&&selected==6);
 strcpy(line,"event home");command(line,reply,sizeof reply);assert(menu==0);
 strcpy(line,"alarm 25 0");command(line,reply,sizeof reply);assert(!strncmp(reply,"ERR",3));assert(cfg.alarm_hour==-1);
 strcpy(line,"alarm 7 30");command(line,reply,sizeof reply);assert(cfg.alarm_hour==7&&cfg.alarm_minute==30);
 strcpy(line,"alarm off");command(line,reply,sizeof reply);assert(cfg.alarm_hour==-1);
 render();unlink(path);puts("PASS core settings, bounds, menu, timers, alarm and renderer");return 0;}
