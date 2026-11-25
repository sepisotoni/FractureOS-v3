#define main fracture_program_main
#include "../device/src/fracture.c"
#undef main
#include <assert.h>
static int lit_pixels(int x,int y,int width,int height){int xx,yy,count=0;uint16_t bg=theme_color(theme.background);for(yy=y;yy<y+height;yy++)for(xx=x;xx<x+width;xx++)if(pixels[yy*W+xx]!=bg)count++;return count;}
int main(void){char root[]="/tmp/fracture-face-test-XXXXXX",path[512],line[128],reply[2048];Face candidate;int i;const char *ids[]={"blue-outline","aqua","lavender","sunset","weather-blue","numerals-duo","roboto","ubuntu","nunito","neon"};
 assert(mkdtemp(root));snprintf(path,sizeof path,"%s/faces",root);assert(!mkdir(path,0700));snprintf(path,sizeof path,"%s/settings.cfg",root);config_path=path;assets_path=getenv("FRACTURE_TEST_ASSETS");if(!assets_path)assets_path="assets/faces";
 for(i=0;i<10;i++){snprintf(line,sizeof line,"face %s",ids[i]);command(line,reply,sizeof reply);assert(!strcmp(reply,"OK\n")&&face_ready);assert(face.show_date&&face.show_seconds);render();}
 strcpy(line,"face-set digit0 319,0,10,100");command(line,reply,sizeof reply);assert(!strncmp(reply,"ERR",3));
 strcpy(line,"face-set hour_color ffaabb");command(line,reply,sizeof reply);assert(!strcmp(reply,"OK\n"));assert(face.hour_color==0xffaabb);assert(face_load()&&face.hour_color==0xffaabb);
 strcpy(line,"face-set atlas ../../bad");command(line,reply,sizeof reply);assert(!strncmp(reply,"ERR",3));
 strcpy(line,"face nunito");command(line,reply,sizeof reply);assert(!strcmp(reply,"OK\n"));
 strcpy(line,"face missing");command(line,reply,sizeof reply);assert(!strncmp(reply,"ERR",3));assert(!strcmp(cfg.face,"nunito"));
 strcpy(line,"face-reset");command(line,reply,sizeof reply);assert(!strcmp(reply,"OK\n"));assert(face.hour_color==0x4be1fa);
 candidate=face;assert(!face_field(&candidate,"digit0","1,1,0,2"));assert(!face_field(&candidate,"digit0","0,0,320,241"));assert(!face_field(&candidate,"time_format","13"));assert(!face_field(&candidate,"hour_color","zzzzzz"));
 {struct tm t;unsigned short saved[W*H];memset(&t,0,sizeof t);t.tm_hour=10;
 face.show_seconds=0;face.hour_only=1;face.blink=1;face_layout(&face,1);face_draw(&t);memcpy(saved,pixels,sizeof saved);
 face_layout(&face,2);face_draw(&t);assert(!memcmp(saved,pixels,sizeof saved));
 t.tm_sec=1;face_draw(&t);assert(!memcmp(saved,pixels,sizeof saved));
 t.tm_min=1;t.tm_sec=0;face_draw(&t);assert(memcmp(saved,pixels,sizeof saved));memcpy(saved,pixels,sizeof saved);
 t.tm_sec=1;face_draw(&t);assert(memcmp(saved,pixels,sizeof saved));
 face.blink=0;face_draw(&t);assert(!memcmp(saved,pixels,sizeof saved));
 }
 menu=PAGE_CLOCK;selected=0;strcpy(line,"event tap");command(line,reply,sizeof reply);assert(menu==PAGE_QUICK);render();assert(lit_pixels(240,0,80,32)>20);assert(lit_pixels(0,35,65,32)==0);assert(lit_pixels(24,66,272,122)>1000);
 strcpy(line,"event power");command(line,reply,sizeof reply);assert(menu==PAGE_EDIT&&edit_parent==PAGE_QUICK);
 free(face_pixels);unlink(config_path);snprintf(path,sizeof path,"%s/faces",root);rmdir(path);rmdir(root);puts("PASS ten face assets, date/seconds defaults, bounds, override persistence, reset, missing face recovery, stacked/horizontal hour-only, colon blinking, and Quick settings navigation");return 0;}
