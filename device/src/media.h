/* Bounded media/notification state. No decoder, account tokens or network worker here. */
static char media_title[81],media_artist[81],media_message[81]="Enable Windows media in app settings";
static char notification[81],notice_app[81],notice_title[81],notice_body[81];
static uint16_t *notice_art;
static char lyrics_line[3][81],lyrics_message[81]="Open lyrics to look up this track";
static int lyrics_plain,lyrics_page;
static int notice_live(void);
static void notice_release_art(void){free(notice_art);notice_art=NULL;}

static double media_at,notice_until,media_action_at;
static int media_playing,media_progress,media_duration,media_volume=-1,media_volume_ok;
static int media_can_play,media_can_prev,media_can_next,media_art_loaded;
static unsigned int media_sequence,media_ack;
static char media_action[16];
static int media_argument;
static uint16_t *media_art;
static int notice_live(void){return cfg.notifications&&notice_until>mono();}
static int media_live(void){return cfg.media&&media_at>0&&mono()-media_at<20;}
static int hex_digit(int c){if(c>='0'&&c<='9')return c-'0';if(c>='a'&&c<='f')return c-'a'+10;return -1;}
static int decode_text(const char *v,char *out,size_t cap){size_t i,n=strlen(v);if(!strcmp(v,"-")){out[0]=0;return 1;}if(n%2||n/2>=cap)return 0;for(i=0;i<n;i+=2){int a=hex_digit(v[i]),b=hex_digit(v[i+1]),c;if(a<0||b<0)return 0;c=a*16+b;if(c<32||c>126)return 0;}for(i=0;i<n;i+=2)out[i/2]=(char)(hex_digit(v[i])*16+hex_digit(v[i+1]));out[n/2]=0;return 1;}
static void media_release_art(void){free(media_art);media_art=NULL;media_art_loaded=0;}
static int media_load_art(void){FILE *f;uint16_t *data;if(menu!=12)return 0;f=fopen("/tmp/fracture-media.rgb","rb");if(!f)return 0;data=malloc(18432);if(!data){fclose(f);return 0;}if(fread(data,1,18432,f)!=18432||fgetc(f)!=EOF){free(data);fclose(f);return 0;}fclose(f);media_release_art();media_art=data;media_art_loaded=1;return 1;}
static void media_send(const char *action,int argument){if(!media_live()){strcpy(media_message,"Windows media is offline");return;}if(media_sequence!=media_ack&&mono()-media_action_at<10)return;snprintf(media_action,sizeof media_action,"%s",action);media_argument=argument;media_sequence++;if(!media_sequence)media_sequence=1;media_action_at=mono();}
static int media_command(const char *name,const char *a,const char *b){int p,pos,dur,vol,vok,play,prev,next;char extra;
 if(!strncmp(name,"media-",6)&&!cfg.media&&strcmp(name,"media-clear"))return 0;
 if(!strcmp(name,"media-text")&&a&&b){char *dst=NULL;if(!strcmp(a,"title"))dst=media_title;else if(!strcmp(a,"artist"))dst=media_artist;else if(!strcmp(a,"message"))dst=media_message;if(!dst)return 0;if(dst==media_title){char next[81];if(!decode_text(b,next,81))return 0;if(strcmp(next,media_title)){memset(lyrics_line,0,sizeof lyrics_line);strcpy(lyrics_message,"Looking up lyrics...");}strcpy(dst,next);return 1;}return decode_text(b,dst,81);}
 if(!strcmp(name,"notice")&&a&&!b){if(!cfg.notifications)return 0;if(!decode_text(a,notification,sizeof notification))return 0;notice_app[0]=notice_title[0]=notice_body[0]=0;notice_release_art();notice_until=notification[0]?mono()+cfg.notice_seconds:0;return 1;}
 if(!strcmp(name,"notice-text")&&cfg.notifications&&a&&b){char *dst=!strcmp(a,"app")?notice_app:!strcmp(a,"title")?notice_title:!strcmp(a,"body")?notice_body:NULL;if(!dst)return 0;return decode_text(b,dst,81);}
 if(!strcmp(name,"notice-art")&&cfg.notifications&&a&&!b){FILE *f;uint16_t *data;if(!strcmp(a,"clear")){notice_release_art();return 1;}if(strcmp(a,"load"))return 0;f=fopen("/tmp/fracture-notice.rgb","rb");if(!f)return 0;data=malloc(4608);if(!data){fclose(f);return 0;}if(fread(data,1,4608,f)!=4608||fgetc(f)!=EOF){free(data);fclose(f);return 0;}fclose(f);notice_release_art();notice_art=data;return 1;}
 if(!strcmp(name,"notice-show")&&cfg.notifications&&a&&!b){char *end;long seconds=strtol(a,&end,10);if(*end||seconds<0||seconds>120)return 0;notification[0]=0;notice_until=mono()+(seconds?seconds:cfg.notice_seconds);return 1;}
 if(!strcmp(name,"media-lyrics")&&a&&b){char *dst=NULL;if(!strcmp(a,"previous"))dst=lyrics_line[0];else if(!strcmp(a,"current"))dst=lyrics_line[1];else if(!strcmp(a,"next"))dst=lyrics_line[2];else if(!strcmp(a,"message"))dst=lyrics_message;if(!dst)return 0;return decode_text(b,dst,81);}
 if(!strcmp(name,"lyrics-state")&&a&&!b){if(strcmp(a,"0")&&strcmp(a,"1"))return 0;lyrics_plain=atoi(a);return 1;}
 if(!strcmp(name,"media-state")&&a&&!b){if(sscanf(a,"%d,%d,%d,%d,%d,%d,%d,%d%c",&p,&pos,&dur,&vol,&vok,&play,&prev,&next,&extra)!=8||p<0||p>1||pos<0||dur<0||dur>86400||pos>dur||vol< -1||vol>100||vok<0||vok>1||play<0||play>1||prev<0||prev>1||next<0||next>1)return 0;media_playing=p;media_progress=pos;media_duration=dur;media_volume=vol;media_volume_ok=vok;media_can_play=play;media_can_prev=prev;media_can_next=next;media_at=mono();return 1;}
 if(!strcmp(name,"media-clear")&&!a){media_at=0;media_ack=media_sequence;media_title[0]=media_artist[0]=0;memset(lyrics_line,0,sizeof lyrics_line);strcpy(lyrics_message,"Waiting for a playing track");media_release_art();return 1;}
 if(!strcmp(name,"media-art")&&a&&!b){if(!strcmp(a,"load"))return media_load_art();if(!strcmp(a,"clear")){media_release_art();return 1;}return 0;}
 if(!strcmp(name,"media-ack")&&a&&!b){char *end;unsigned long n=strtoul(a,&end,10);if(*end||n!=media_sequence)return 0;media_ack=(unsigned int)n;return 1;}
 return 0;}
static void duration_text(char *s,size_t cap,int seconds){if(seconds<0)seconds=0;if(seconds>=3600)snprintf(s,cap,"%d:%02d:%02d",seconds/3600,seconds/60%60,seconds%60);else snprintf(s,cap,"%02d:%02d",seconds/60,seconds%60);}
static int metrics_live(void){return cfg.telemetry&&stats_at>0&&mono()-stats_at<10;}
static int temperature_live(void){return cfg.telemetry&&temp_at>0&&mono()-temp_at<10&&cpu_temp>=-500;}
static int metric_count(void){return cfg.profile==1&&metrics_live()?(cpu>=0)+(ram>=0)+temperature_live()+(net_down>=0||net_up>=0):0;}
/* A bottom dock shares horizontal space. Activity never becomes a stack. */
static int activity_count(void){return (timer_end>0)+(watch_start>0||watch_elapsed>0)+(cfg.alarm_hour>=0);}
static int dock_extras(void){return (media_live()&&media_title[0])||weather_visible();}
static int dock_height(void){if(!metric_count()&&!dock_extras())return 0;if(metric_count())return media_live()&&media_title[0]&&weather_visible()?72:54;return 54;}
static int status_height(void){if(notice_live())return 96;return (activity_count()?26:0)+dock_height()+((activity_count()||dock_height())?8:0);}
static int measured(const char *s){return ui_font?ui_width(s,1):(int)strlen(s)*6;}
static void activity_draw(int top){char labels[3][24];int kinds[3],count=0,i,total=0,x,widths[3],gap;
 if(timer_end>0){int seconds=(int)(timer_end-mono()+0.999);duration_text(labels[count],24,seconds);if(seconds>=3600)snprintf(labels[count],24,"%dh%02d",seconds/3600,seconds/60%60);kinds[count++]=3;}
 if(watch_start>0||watch_elapsed>0){int seconds=(int)(watch_elapsed+(watch_start?mono()-watch_start:0));duration_text(labels[count],24,seconds);if(seconds>=3600)snprintf(labels[count],24,"%dh%02d",seconds/3600,seconds/60%60);kinds[count++]=5;}
 if(cfg.alarm_hour>=0){snprintf(labels[count],24,"%02d:%02d",cfg.alarm_hour,cfg.alarm_minute);kinds[count++]=8;}
 for(i=0;i<count;i++){widths[i]=27+measured(labels[i]);total+=widths[i];}gap=count>1?(W-24-total)/(count-1):0;if(gap<8)gap=8;x=count==1?(W-total)/2:12;
 for(i=0;i<count;i++){icon(x,top,kinds[i],theme_color(theme.accent),theme_color(theme.background));text_fit(x+27,top+2,labels[i],widths[i]-25,1,theme_color(theme.foreground));x+=widths[i]+gap;}}
static void notice_draw(int top){int x,y,tx=24,tw=272;uint16_t bg=appearance_day?rgb(225,228,232):rgb(34,37,42),fg=theme_color(theme.foreground),muted=theme_color(theme.muted);rounded(8,top+3,304,H-top-7,15,bg);
 if(notice_art){for(y=0;y<48;y++)for(x=0;x<48;x++)pixels[(top+22+y)*W+20+x]=notice_art[y*48+x];tx=80;tw=218;}else{rounded(20,top+13,20,20,6,theme_color(theme.surface));icon(19,top+11,8,theme_color(theme.accent),bg);tx=48;tw=246;}
 if(notification[0]){text_lines(tx,top+15,notification,tw,3,1,fg);return;}
 text_fit(tx,top+10,notice_app[0]?notice_app:"Notification",tw,1,muted);text_fit(tx,top+30,notice_title,tw,1,fg);text_lines(tx,top+50,notice_body,tw,2,1,fg);}
static void status_draw(int top){int values[4],icons[4]={1,10,16,15},i,slot=0,metrics=metric_count(),extras=dock_extras(),panel=metrics&&extras?186:W,dh=dock_height(),y=H-dh-3;char value[32];uint16_t fg=theme_color(theme.foreground),accent=theme_color(theme.accent),bg=theme_color(theme.background);
 if(notice_live()){notice_draw(top);return;}rect(12,top,296,1,theme_color(theme.surface));if(activity_count())activity_draw(top+5);
 if(metrics){values[0]=cpu;values[1]=ram;values[2]=temperature_live()?cpu_temp:-1000;values[3]=net_down>net_up?net_down:net_up;
 for(i=0;i<4;i++)if(values[i]>=0||(i==2&&temperature_live())){int cell=panel/2,x=10+(slot%2)*cell,yy=y+(dh-48)/2+(slot/2)*25;icon(x,yy,icons[i],accent,bg);if(i==3){if(values[i]>=1024)snprintf(value,sizeof value,"%d.%dM",values[i]/1024,(values[i]%1024)*10/1024);else snprintf(value,sizeof value,"%dK/s",values[i]);}else if(i==2)snprintf(value,sizeof value,"%s%dC",temp_pc?"P ":"",values[i]/10);else snprintf(value,sizeof value,"%d%%",values[i]);text_fit(x+27,yy+2,value,cell-31,1,fg);slot++;}}
 if(extras){int left=metrics?panel+8:14,width=metrics?W-left-8:W-28,row=y+4;int music=media_live()&&media_title[0];if(!metrics&&music&&weather_visible())width=166;if(metrics)rounded(panel,y+2,1,dh-5,0,theme_color(theme.surface));
 if(music){spotify_mark(left,row+1,17);text_fit(left+23,row,media_title,width-23,1,fg);text_fit(left,row+20,media_artist,width,1,theme_color(theme.muted));row+=weather_visible()?43:40;}
 if(weather_visible()){if(!metrics&&music){left=198;width=110;row=y+4;}weather_icon(left,row,weather_code,accent,bg);snprintf(value,sizeof value,"%s%d C",weather_temp<0?"-":"",abs(weather_temp)/10);text_fit(left+28,row+2,value,width-28,1,fg);if(!music||!metrics)text_fit(left,row+23,weather_label(weather_code),width,1,theme_color(theme.muted));}}
}
static void media_draw(void){int x,y,progress;char s[40];struct tm t;uint16_t fg=theme_color(theme.foreground),muted=theme_color(theme.muted),accent=theme_color(theme.accent),bg=theme_color(theme.background);
 spotify_mark(17,6,26);localtime_now(&t);strftime(s,sizeof s,"%H:%M",&t);text(W-16-measured(s),10,s,1,fg);
 if(media_art)for(y=0;y<96;y++)for(x=0;x<96;x++)pixels[(44+y)*W+16+x]=media_art[y*96+x];else{rounded(16,44,96,96,14,theme_color(theme.surface));media_symbol(45,74,36,SYM_SPOTIFY,accent,bg);}
 text_lines(126,44,media_live()&&media_title[0]?media_title:"No active track",180,2,2,fg);text_fit(126,96,media_artist,180,1,muted);if(strcmp(media_message,"Spotify on Windows"))text_fit(126,118,media_message,180,1,muted);
 progress=media_progress+(media_live()&&media_playing?(int)(mono()-media_at):0);if(progress>media_duration)progress=media_duration;rounded(16,151,288,3,1,theme_color(theme.surface));if(media_duration>0)rect(16,151,288*progress/media_duration,3,accent);duration_text(s,sizeof s,progress);text(16,158,s,1,muted);duration_text(s,sizeof s,media_duration);text(W-16-measured(s),158,s,1,muted);
 {int symbols[5]={SYM_PREV,media_playing?SYM_PAUSE:SYM_PLAY,SYM_NEXT,SYM_VOLUME,SYM_MORE};for(x=0;x<5;x++){uint16_t c=x==selected?fg:muted;rounded(12+x*60,184,55,31,11,x==selected?theme_color(theme.surface):bg);media_symbol(28+x*60,188,24,symbols[x],c,x==selected?theme_color(theme.surface):bg);}}
 {const char *labels[]={"Previous",media_playing?"Pause":"Play","Next","Volume","More"};center(221,labels[selected],1,muted);}}
static void lyrics_draw(void){struct tm t;char s[16];uint16_t fg=theme_color(theme.foreground),muted=theme_color(theme.muted);spotify_mark(16,8,24);text(50,10,"Lyrics",2,fg);localtime_now(&t);strftime(s,sizeof s,"%H:%M",&t);text(W-16-measured(s),12,s,1,fg);text_fit(16,47,media_title,288,1,muted);
 if(!lyrics_line[0][0]&&!lyrics_line[1][0]&&!lyrics_line[2][0]){text_lines(18,100,lyrics_message,284,3,1,muted);}else{text_fit(18,82,lyrics_line[0],284,1,muted);text_lines(18,112,lyrics_line[1],284,2,2,fg);text_fit(18,178,lyrics_line[2],284,1,muted);}
 center(220,lyrics_plain?"Tap: next / double: previous":"Synced lyrics / hold: back",1,muted);}
