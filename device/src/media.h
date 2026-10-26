/* Bounded media/notification state. No decoder, account tokens or network worker here. */
static char media_title[81],media_artist[81],media_message[81]="Enable Windows media in app settings";
static char notification[81];
static double media_at,notice_until,media_action_at;
static int media_playing,media_progress,media_duration,media_volume=-1,media_volume_ok;
static int media_can_play,media_can_prev,media_can_next,media_art_loaded;
static unsigned int media_sequence,media_ack;
static char media_action[16];
static int media_argument;
static uint16_t *media_art;
static int media_live(void){return cfg.media&&media_at>0&&mono()-media_at<20;}
static int hex_digit(int c){if(c>='0'&&c<='9')return c-'0';if(c>='a'&&c<='f')return c-'a'+10;return -1;}
static int decode_text(const char *v,char *out,size_t cap){size_t i,n=strlen(v);if(!strcmp(v,"-")){out[0]=0;return 1;}if(n%2||n/2>=cap)return 0;for(i=0;i<n;i+=2){int a=hex_digit(v[i]),b=hex_digit(v[i+1]),c;if(a<0||b<0)return 0;c=a*16+b;if(c<32||c>126)return 0;}for(i=0;i<n;i+=2)out[i/2]=(char)(hex_digit(v[i])*16+hex_digit(v[i+1]));out[n/2]=0;return 1;}
static void media_release_art(void){free(media_art);media_art=NULL;media_art_loaded=0;}
static int media_load_art(void){FILE *f;uint16_t *data;if(menu!=12)return 0;f=fopen("/tmp/fracture-media.rgb","rb");if(!f)return 0;data=malloc(18432);if(!data){fclose(f);return 0;}if(fread(data,1,18432,f)!=18432||fgetc(f)!=EOF){free(data);fclose(f);return 0;}fclose(f);media_release_art();media_art=data;media_art_loaded=1;return 1;}
static void media_send(const char *action,int argument){if(!media_live()){strcpy(media_message,"Windows media is offline");return;}if(media_sequence!=media_ack&&mono()-media_action_at<10)return;snprintf(media_action,sizeof media_action,"%s",action);media_argument=argument;media_sequence++;if(!media_sequence)media_sequence=1;media_action_at=mono();}
static int media_command(const char *name,const char *a,const char *b){int p,pos,dur,vol,vok,play,prev,next;char extra;
 if(!strncmp(name,"media-",6)&&!cfg.media&&strcmp(name,"media-clear"))return 0;
 if(!strcmp(name,"media-text")&&a&&b){char *dst=NULL;if(!strcmp(a,"title"))dst=media_title;else if(!strcmp(a,"artist"))dst=media_artist;else if(!strcmp(a,"message"))dst=media_message;if(!dst)return 0;return decode_text(b,dst,81);}
 if(!strcmp(name,"notice")&&a&&!b){if(!cfg.notifications)return 0;if(!decode_text(a,notification,sizeof notification))return 0;notice_until=notification[0]?mono()+15:0;return 1;}
 if(!strcmp(name,"media-state")&&a&&!b){if(sscanf(a,"%d,%d,%d,%d,%d,%d,%d,%d%c",&p,&pos,&dur,&vol,&vok,&play,&prev,&next,&extra)!=8||p<0||p>1||pos<0||dur<0||dur>86400||pos>dur||vol< -1||vol>100||vok<0||vok>1||play<0||play>1||prev<0||prev>1||next<0||next>1)return 0;media_playing=p;media_progress=pos;media_duration=dur;media_volume=vol;media_volume_ok=vok;media_can_play=play;media_can_prev=prev;media_can_next=next;media_at=mono();return 1;}
 if(!strcmp(name,"media-clear")&&!a){media_at=0;media_ack=media_sequence;media_title[0]=media_artist[0]=0;media_release_art();return 1;}
 if(!strcmp(name,"media-art")&&a&&!b){if(!strcmp(a,"load"))return media_load_art();if(!strcmp(a,"clear")){media_release_art();return 1;}return 0;}
 if(!strcmp(name,"media-ack")&&a&&!b){char *end;unsigned long n=strtoul(a,&end,10);if(*end||n!=media_sequence)return 0;media_ack=(unsigned int)n;return 1;}
 return 0;}
static void duration_text(char *s,size_t cap,int seconds){if(seconds<0)seconds=0;if(seconds>=3600)snprintf(s,cap,"%d:%02d:%02d",seconds/3600,seconds/60%60,seconds%60);else snprintf(s,cap,"%02d:%02d",seconds/60,seconds%60);}
static int metrics_live(void){return cfg.telemetry&&stats_at>0&&mono()-stats_at<10;}
static int metric_count(void){return cfg.profile==1&&metrics_live()?(cpu>=0)+(ram>=0)+(disk_usage>=0)+(net_down>=0||net_up>=0):0;}
static int status_rows(void){int slots=(timer_end>0)+(watch_start>0||watch_elapsed>0)+(cfg.alarm_hour>=0),count=(slots+1)/2;count+=(metric_count()+1)/2;if(weather_visible())count++;if(media_live()&&media_title[0])count++;if(notice_until>mono())count++;return count;}
static void status_draw(int top){int row=top+7;char s[128],a[24];uint16_t fg=theme_color(theme.foreground),accent=theme_color(theme.accent);rect(18,top,284,1,theme_color(theme.surface));
 if(timer_end>0||watch_start>0||watch_elapsed>0||cfg.alarm_hour>=0){int count=(timer_end>0)+(watch_start>0||watch_elapsed>0)+(cfg.alarm_hour>=0),slot=0,width=count==1?296:148;
 if(timer_end>0){duration_text(a,sizeof a,(int)(timer_end-mono()+0.999));snprintf(s,sizeof s,"%s",a);icon(12+(slot%2)*width,row,3,accent,0);text_fit(38+(slot%2)*width,row,s,width-29,1,fg);slot++;if(slot==2&&count>2)row+=27;}
 if(watch_start>0||watch_elapsed>0){duration_text(a,sizeof a,(int)(watch_elapsed+(watch_start?mono()-watch_start:0)));icon(12+(slot%2)*width,row,5,accent,0);text_fit(38+(slot%2)*width,row,a,width-29,1,fg);slot++;if(slot==2&&count>2)row+=27;}
 if(cfg.alarm_hour>=0){snprintf(a,sizeof a,"%02d:%02d",cfg.alarm_hour,cfg.alarm_minute);icon(12+(slot%2)*width,row,8,accent,0);text_fit(38+(slot%2)*width,row,a,width-29,1,fg);slot++;if(slot==2&&count>2)row+=27;}
 row+=27;}
 if(metric_count()){int values[4],icons[4]={1,10,14,15},i,slot=0;values[0]=cpu;values[1]=ram;values[2]=disk_usage;values[3]=net_down>net_up?net_down:net_up;
 for(i=0;i<4;i++)if(values[i]>=0){int x=16+(slot%2)*152;icon(x,row,icons[i],accent,0);if(i==3){if(values[i]>=1024)snprintf(a,sizeof a,"%d.%d M/s",values[i]/1024,(values[i]%1024)*10/1024);else snprintf(a,sizeof a,"%d K/s",values[i]);}else snprintf(a,sizeof a,"%d%%",values[i]);text_fit(x+29,row,a,119,1,fg);slot++;if(slot%2==0)row+=27;}if(slot%2)row+=27;}
 if(weather_visible()){weather_icon(16,row,weather_code,accent,theme_color(theme.background));snprintf(s,sizeof s,"%s%d C / %s",weather_temp<0?"-":"",abs(weather_temp)/10,weather_label(weather_code));text_fit(47,row,s,259,1,fg);row+=27;}

 if(media_live()&&media_title[0]){snprintf(s,sizeof s,"Spotify / %s",media_title);text_fit(16,row,s,288,1,fg);row+=27;}
 if(notice_until>mono())text_fit(16,row,notification,288,1,accent);}
static void media_draw(void){int x,y,progress;char s[40];uint16_t fg=theme_color(theme.foreground),muted=theme_color(theme.muted),accent=theme_color(theme.accent),bg=theme_color(theme.background);
 text(18,7,"Spotify",2,rgb(30,215,96));if(media_art)for(y=0;y<96;y++)for(x=0;x<96;x++)pixels[(44+y)*W+16+x]=media_art[y*96+x];else{rounded(16,44,96,96,14,theme_color(theme.surface));icon(51,79,5,accent,theme_color(theme.surface));}
 text_fit(126,45,media_live()&&media_title[0]?media_title:"No active track",180,2,fg);text_fit(126,76,media_artist,180,1,muted);text_fit(126,99,media_message,180,1,muted);
 progress=media_progress+(media_live()&&media_playing?(int)(mono()-media_at):0);if(progress>media_duration)progress=media_duration;rounded(16,151,288,3,1,theme_color(theme.surface));if(media_duration>0)rect(16,151,288*progress/media_duration,3,accent);duration_text(s,sizeof s,progress);text(16,158,s,1,muted);duration_text(s,sizeof s,media_duration);text(266,158,s,1,muted);
 {const char *labels[]={"Prev",media_playing?"Pause":"Play","Next","Vol","More"};for(x=0;x<5;x++){uint16_t c=x==selected?fg:muted;rounded(12+x*60,185,55,28,10,x==selected?theme_color(theme.surface):bg);text_fit(17+x*60,190,labels[x],46,1,c);}}
 center(221,"Tap: browse / power: select",1,muted);}
