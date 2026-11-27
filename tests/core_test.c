#define main fracture_program_main
#include "../device/src/fracture.c"
#undef main
#include <assert.h>
int main(void){static const char *preset_faces[]={"blue-outline","sunset","aqua","neon"},*preset_themes[]={"blue","amber","teal","midnight"},*preset_atlases[]={"digit-atlas.pgm","digit-atlas-solid.pgm","digit-atlas-nunito.pgm"};static const int preset_layouts[]={1,1,2,2};char root[]="/tmp/fracture-core-test-XXXXXX",path[256],face_dir[256],assets_dir[256],theme_dir[256],asset_path[256],source[512],cwd[256],line[128],reply[2048];int i;assert(mkdtemp(root));assert(getcwd(cwd,sizeof cwd));snprintf(path,sizeof path,"%s/settings.cfg",root);snprintf(face_dir,sizeof face_dir,"%s/faces",root);snprintf(assets_dir,sizeof assets_dir,"%s/assets",root);snprintf(theme_dir,sizeof theme_dir,"%s/themes",root);assert(!mkdir(face_dir,0700));assert(!mkdir(assets_dir,0700));assert(!mkdir(theme_dir,0700));for(i=0;i<4;i++){snprintf(source,sizeof source,"%s/assets/faces/%s.face",cwd,preset_faces[i]);snprintf(asset_path,sizeof asset_path,"%s/%s.face",assets_dir,preset_faces[i]);assert(!symlink(source,asset_path));snprintf(source,sizeof source,"%s/assets/themes/%s.theme",cwd,preset_themes[i]);snprintf(asset_path,sizeof asset_path,"%s/%s.theme",theme_dir,preset_themes[i]);assert(!symlink(source,asset_path));}for(i=0;i<3;i++){snprintf(source,sizeof source,"%s/assets/faces/%s",cwd,preset_atlases[i]);snprintf(asset_path,sizeof asset_path,"%s/%s",assets_dir,preset_atlases[i]);assert(!symlink(source,asset_path));}config_path=path;assets_path=assets_dir;
 assert(!strcmp(weather_label(0),"Clear"));assert(!strcmp(weather_label(1),"Mostly clear"));assert(!strcmp(weather_label(2),"Partly cloudy"));assert(!strcmp(weather_label(3),"Overcast"));assert(!strcmp(weather_label(57),"Heavy freezing drizzle"));assert(!strcmp(weather_label(96),"Thunderstorm with hail"));assert(!strcmp(weather_label(98),"Unknown"));
 strcpy(line,"profile bedside");command(line,reply,sizeof reply);assert(!strcmp(reply,"OK\n"));assert(cfg.profile==2);
 strcpy(line,"brightness 101");command(line,reply,sizeof reply);assert(!strncmp(reply,"ERR",3));assert(cfg.brightness==30);
 strcpy(line,"offset -720");command(line,reply,sizeof reply);assert(cfg.offset==-720);
 cfg.offset=0;load();assert(cfg.offset==-720);
 strcpy(line,"timer 3");command(line,reply,sizeof reply);assert(timer_end>mono());
 strcpy(line,"timer 0");command(line,reply,sizeof reply);assert(timer_end==0);
 strcpy(line,"event power");command(line,reply,sizeof reply);assert(menu==1);
 strcpy(line,"event tap");command(line,reply,sizeof reply);assert(selected==1);
 strcpy(line,"event power");command(line,reply,sizeof reply);assert(menu==PAGE_CLOCK_TOOLS);
 strcpy(line,"event long");command(line,reply,sizeof reply);assert(menu==PAGE_APPS);
 strcpy(line,"event home");command(line,reply,sizeof reply);assert(menu==0);
 strcpy(line,"ui-timeout menu 3");command(line,reply,sizeof reply);assert(!strcmp(reply,"OK\n"));
 page_open(PAGE_APPS);menu_activity=10;menu_timeout_tick(12);assert(menu==PAGE_APPS);
 touch_down=1;menu_timeout_tick(13);assert(menu==PAGE_APPS);
 touch_held=1;menu_timeout_tick(13);assert(menu==PAGE_CLOCK);touch_down=touch_held=0;
 page_open(PAGE_LYRICS);menu_activity=10;menu_timeout_tick(20);assert(menu==PAGE_LYRICS);
 strcpy(line,"ui-timeout menu 0");command(line,reply,sizeof reply);assert(!strcmp(reply,"OK\n"));
 page_open(PAGE_APPS);menu_activity=10;menu_timeout_tick(20);assert(menu==PAGE_APPS);
 menu=PAGE_APPS;selected=4;strcpy(line,"event power");command(line,reply,sizeof reply);assert(menu==PAGE_AGENDA);
 strcpy(line,"agenda-item 0,31343a3330,4d656574696e67,4f6666696365");command(line,reply,sizeof reply);assert(!strcmp(reply,"OK\n"));assert(agenda_count==1&&!strcmp(agenda_title[0],"Meeting")&&!strcmp(agenda_time[0],"14:30")&&!strcmp(agenda_location[0],"Office"));
 strcpy(line,"status");command(line,reply,sizeof reply);assert(strstr(reply,"\"agenda_view\":true"));
 strcpy(line,"event power");command(line,reply,sizeof reply);assert(menu==PAGE_APPS);
 strcpy(line,"agenda-clear");command(line,reply,sizeof reply);assert(agenda_count==0);
 menu=PAGE_SETTINGS;selected=11;strcpy(line,"event power");command(line,reply,sizeof reply);assert(menu==PAGE_PRESETS&&page_count()==4);
 for(i=0;i<4;i++){menu=PAGE_PRESETS;selected=i;strcpy(line,"event power");command(line,reply,sizeof reply);assert(menu==PAGE_CLOCK&&!strcmp(cfg.face,preset_faces[i])&&!strcmp(cfg.theme_name,preset_themes[i])&&face.layout==preset_layouts[i]);}
 strcpy(line,"alarm 25 0");command(line,reply,sizeof reply);assert(!strncmp(reply,"ERR",3));assert(cfg.alarm_hour==-1);
 strcpy(line,"alarm 7 30");command(line,reply,sizeof reply);assert(cfg.alarm_hour==7&&cfg.alarm_minute==30);
 strcpy(line,"alarm off");command(line,reply,sizeof reply);assert(cfg.alarm_hour==-1);
 render();for(i=0;i<4;i++){snprintf(asset_path,sizeof asset_path,"%s/%s.face",assets_dir,preset_faces[i]);unlink(asset_path);snprintf(asset_path,sizeof asset_path,"%s/%s.face",face_dir,preset_faces[i]);unlink(asset_path);snprintf(asset_path,sizeof asset_path,"%s/%s.theme",theme_dir,preset_themes[i]);unlink(asset_path);}for(i=0;i<3;i++){snprintf(asset_path,sizeof asset_path,"%s/%s",assets_dir,preset_atlases[i]);unlink(asset_path);}assert(!rmdir(theme_dir));assert(!rmdir(assets_dir));unlink(path);assert(!rmdir(face_dir));assert(!rmdir(root));puts("PASS core settings, bounds, four presets, menu, timers, alarm and renderer");return 0;}
