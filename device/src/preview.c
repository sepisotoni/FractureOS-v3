/* Compiled as an optional Windows DLL. No device I/O or networking: the same
 * home drawing code consumes authenticated model/asset data on the PC. */
#define FRACTURE_RENDER_ONLY
#include "fracture.c"
#ifdef _WIN32
#define EXPORT __declspec(dllexport)
#else
#define EXPORT __attribute__((visibility("default")))
#endif
EXPORT void FracturePreviewClose(void){free(face_pixels);face_pixels=NULL;free(ui_font);ui_font=NULL;notice_release_art();face_ready=0;}
EXPORT int FracturePreviewAssets(const unsigned char *atlas,int atlas_size,const unsigned char *font_data,int font_size){unsigned char *a,*f=NULL;if(!atlas||atlas_size!=170000||(font_size!=0&&font_size!=45699))return 0;a=malloc(170000);if(!a)return 0;memcpy(a,atlas,170000);if(font_size){if(!font_data||memcmp(font_data,"FUI1",4)){free(a);return 0;}f=malloc(45699);if(!f){free(a);return 0;}memcpy(f,font_data,45699);}free(face_pixels);free(ui_font);face_pixels=a;ui_font=f;face_ready=1;return 1;}
EXPORT int FracturePreviewNotice(const unsigned char *data,int size){notice_release_art();if(!size)return 1;if(size!=4608||!data)return 0;notice_art=malloc(4608);if(!notice_art)return 0;memcpy(notice_art,data,4608);return 1;}
static double preview_at(double age){return age>=100000?0:1000-age;}
EXPORT int FracturePreviewRender(const char *model,unsigned int elapsed_ms,unsigned char *out,int size){char copy[8192],*line,*equal;int i;double epoch=0;if(!model||strlen(model)>=sizeof copy||strncmp(model,"FPR1\n",5)||!strstr(model,"\nEND\n")||!out||size!=W*H*4||!face_pixels)return 0;strcpy(copy,model);preview_mono=1000+(double)elapsed_ms/1000;memset(&face,0,sizeof face);memset(&theme,0,sizeof theme);menu=0;
 for(line=strtok(copy,"\n");line;line=strtok(NULL,"\n")){double n;equal=strchr(line,'=');if(!equal)continue;*equal++=0;n=strtod(equal,NULL);
 #define SET(k,v) if(!strcmp(line,k)){v=n;continue;}
 SET("now",epoch);SET("offset",cfg.offset);SET("profile",cfg.profile);SET("appearance",cfg.appearance);SET("telemetry",cfg.telemetry);SET("weather",cfg.weather);SET("media",cfg.media);SET("notifications",cfg.notifications);SET("alarm_hour",cfg.alarm_hour);SET("alarm_minute",cfg.alarm_minute);SET("alert",alert);
 if(!strcmp(line,"face")){if(!identifier(equal)||strlen(equal)>31)return 0;strcpy(cfg.face,equal);continue;}
 if(!strncmp(line,"face.",5)){char value[80];if(strlen(equal)>=sizeof value)return 0;strcpy(value,equal);if(!strcmp(line,"face.atlas")||strstr(line,"digit")||strstr(line,"color")||!strcmp(line,"face.background")){}else snprintf(value,sizeof value,"%d",(int)n);if(!face_field(&face,line+5,value))return 0;continue;}
 if(!strncmp(line,"theme.",6)){char value[80];if(strlen(equal)>=sizeof value)return 0;strcpy(value,equal);if(!strcmp(line,"theme.face_colors"))snprintf(value,sizeof value,"%d",(int)n);if(!theme_field(&theme,line+6,value))return 0;continue;}
 SET("cpu",cpu);SET("ram",ram);SET("cpu_temp",cpu_temp);SET("temp_pc",temp_pc);SET("net_down",net_down);SET("net_up",net_up);SET("media_playing",media_playing);SET("media_position",media_position_ms);SET("media_duration",media_duration);SET("weather_code",weather_code);SET("weather_temp",weather_temp);SET("weather_unusual",weather_unusual);SET("battery_state",battery_state);
 if(!strcmp(line,"timer_left")){timer_end=n>0?1000+n:0;continue;}if(!strcmp(line,"watch_elapsed")){watch_elapsed=n;continue;}if(!strcmp(line,"watch_running")){watch_start=n?1000:0;continue;}if(!strcmp(line,"notice_left")){notice_until=n>0?1000+n:0;continue;}
 if(!strcmp(line,"stats_age")){stats_at=preview_at(n);continue;}if(!strcmp(line,"temp_age")){temp_at=preview_at(n);continue;}if(!strcmp(line,"media_age")){media_at=preview_at(n);continue;}if(!strcmp(line,"weather_age")){weather_at=preview_at(n);continue;}if(!strcmp(line,"link_age")){link_at=preview_at(n);continue;}
 #define TEXT(k,v) if(!strcmp(line,k)){if(!*equal)v[0]=0;else if(!decode_text(equal,v,sizeof v))return 0;continue;}
 TEXT("media_title",media_title);TEXT("media_artist",media_artist);TEXT("notification",notification);TEXT("notice_app",notice_app);TEXT("notice_title",notice_title);TEXT("notice_body",notice_body);
 #undef TEXT
 #undef SET
 }
 if(epoch<1700000000||epoch>2147483647)return 0;wall_override=(time_t)(epoch+(double)elapsed_ms/1000);media_clock_at=1000;render();for(i=0;i<W*H;i++){out[i*4]=(pixels[i]&31)*255/31;out[i*4+1]=((pixels[i]>>5)&63)*255/63;out[i*4+2]=((pixels[i]>>11)&31)*255/31;out[i*4+3]=255;}return 1;}
