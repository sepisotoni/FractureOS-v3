/* A bounded home-screen model, never a framebuffer capture. Assets are exported
 * separately by hash, once per change, for the optional desktop renderer. */
#include <stdarg.h>
static void model_add(char *out,size_t cap,const char *format,...){size_t used=strlen(out);va_list args;if(used>=cap)return;va_start(args,format);vsnprintf(out+used,cap-used,format,args);va_end(args);}
static void model_text(char *out,size_t cap,const char *key,const char *value){char hex[163];size_t i;for(i=0;i<80&&value[i];i++)sprintf(hex+i*2,"%02x",(unsigned char)value[i]);hex[i*2]=0;model_add(out,cap,"%s=%s\n",key,hex);}
static double model_age(double at){return at>0?mono()-at:100000;}
static void home_model(char *out,size_t cap){int i;struct timeval tv;battery_refresh();gettimeofday(&tv,NULL);out[0]=0;model_add(out,cap,"FPR1\nnow=%.3f\n",wall_override?(double)wall_override:tv.tv_sec+tv.tv_usec/1e6);
 #define NUM(k,v) model_add(out,cap,#k "=%.3f\n",(double)(v))
 NUM(offset,cfg.offset);NUM(profile,cfg.profile);NUM(appearance,cfg.appearance);NUM(telemetry,cfg.telemetry);NUM(weather,cfg.weather);NUM(media,cfg.media);NUM(notifications,cfg.notifications);NUM(alarm_hour,cfg.alarm_hour);NUM(alarm_minute,cfg.alarm_minute);NUM(alert,alert);
 model_add(out,cap,"face=%s\ntheme_name=%s\natlas_hash=%lu\nfont_hash=%lu\nnotice_hash=%lu\n",cfg.face,cfg.theme_name,atlas_hash,ui_font_hash,notice_art?content_hash((const unsigned char*)notice_art,4608):0UL);
 NUM(timer_left,timer_end>mono()?timer_end-mono():0);NUM(watch_elapsed,watch_elapsed+(watch_start?mono()-watch_start:0));NUM(watch_running,watch_start>0);NUM(stats_age,model_age(stats_at));NUM(temp_age,model_age(temp_at));NUM(cpu,cpu);NUM(ram,ram);NUM(cpu_temp,cpu_temp);NUM(temp_pc,temp_pc);NUM(net_down,net_down);NUM(net_up,net_up);
 NUM(media_age,model_age(media_at));NUM(media_playing,media_playing);NUM(media_position,playback_ms());NUM(media_duration,media_duration);NUM(weather_age,model_age(weather_at));NUM(weather_code,weather_code);NUM(weather_temp,weather_temp);NUM(weather_unusual,weather_unusual);NUM(notice_left,notice_live()?notice_until-mono():0);NUM(link_age,model_age(link_at));NUM(battery_state,battery_state);
 model_text(out,cap,"media_title",media_title);model_text(out,cap,"media_artist",media_artist);model_text(out,cap,"notification",notification);model_text(out,cap,"notice_app",notice_app);model_text(out,cap,"notice_title",notice_title);model_text(out,cap,"notice_body",notice_body);
 model_add(out,cap,"face.atlas=%s\nface.hour_color=%06lx\nface.minute_color=%06lx\nface.background=%06lx\n",face.atlas,face.hour_color,face.minute_color,face.background);
 #define FACE(k) NUM(face.k,face.k)
 FACE(time_format);FACE(show_date);FACE(show_ampm);FACE(colon);FACE(date_x);FACE(date_y);FACE(date_scale);FACE(blink);FACE(hour_only);FACE(show_seconds);FACE(weight);FACE(colon_x);FACE(colon_y);FACE(colon_gap);FACE(colon_size);
 /* digit geometry already includes layout; do not reapply its presets. */
 for(i=0;i<4;i++)model_add(out,cap,"face.digit%d=%d,%d,%d,%d\n",i,face.digit[i][0],face.digit[i][1],face.digit[i][2],face.digit[i][3]);
 #define THEME(k) model_add(out,cap,"theme." #k "=%06lx\n",theme.k)
 THEME(background);THEME(foreground);THEME(muted);THEME(accent);THEME(surface);THEME(hour_color);THEME(minute_color);NUM(theme.face_colors,theme.face_colors);
 model_add(out,cap,"END\n");
 #undef THEME
 #undef FACE
 #undef NUM
}
static void preview_asset(const char *kind,const char *offset,char *out,size_t cap){const unsigned char *data=NULL;size_t size=0,count,i;char *end;long at=strtol(offset,&end,10);if(*end||at<0||at%8192)return;
 if(!strcmp(kind,"atlas")){data=face_pixels;size=170000;}else if(!strcmp(kind,"font")){data=ui_font;size=45699;}else if(!strcmp(kind,"notice")){data=(const unsigned char*)notice_art;size=4608;}if(!data||(size_t)at>=size)return;count=size-at;if(count>8192)count=8192;if(count*2+2>cap)return;
 for(i=0;i<count;i++)sprintf(out+i*2,"%02x",data[at+i]);out[count*2]='\n';out[count*2+1]=0;}
