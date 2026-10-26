/* Small page model. A numeric editor is entered explicitly before taps change values. */
enum {PAGE_CLOCK, PAGE_APPS, PAGE_TIMER, PAGE_TIMER_CUSTOM, PAGE_WATCH, PAGE_ALARM,
 PAGE_ALARM_CUSTOM, PAGE_SETTINGS, PAGE_MODES, PAGE_POWER, PAGE_CONFIRM, PAGE_MEDIA,
 PAGE_PLAYER, PAGE_MEDIA_MORE, PAGE_EDIT, PAGE_INFO};
static int saved_selection[16],edit_parent,info_parent,edit_field,edit_value,edit_min,edit_max,edit_step;
static int custom_timer[3]={0,25,0},custom_alarm[2]={7,0},pending_power;
static double watch_lap;
static const char *edit_label="Value",*info_text="";
static void power_action(int action);
static const char *page_title(void){switch(menu){case PAGE_APPS:return "Apps";case PAGE_TIMER:return "Timer";case PAGE_TIMER_CUSTOM:return "Custom timer";case PAGE_WATCH:return "Stopwatch";case PAGE_ALARM:return "Alarm";case PAGE_ALARM_CUSTOM:return "Custom alarm";case PAGE_SETTINGS:return "Settings";case PAGE_MODES:return "Clock mode";case PAGE_POWER:return "Power";case PAGE_CONFIRM:return pending_power==1?"Suspend?":pending_power==2?"Restart?":"Shut down?";case PAGE_MEDIA:return "Media";case PAGE_MEDIA_MORE:return "Spotify options";case PAGE_EDIT:return edit_label;default:return "Information";}}
static int page_count(void){switch(menu){case PAGE_APPS:return 6;case PAGE_TIMER:return 6;case PAGE_TIMER_CUSTOM:return 4;case PAGE_WATCH:return 4;case PAGE_ALARM:return 5;case PAGE_ALARM_CUSTOM:return 3;case PAGE_SETTINGS:return 6;case PAGE_MODES:return 3;case PAGE_POWER:return 3;case PAGE_CONFIRM:return 2;case PAGE_MEDIA:return 1;case PAGE_PLAYER:return 5;case PAGE_MEDIA_MORE:return 3;default:return 1;}}
static void page_open(int page){if(page==PAGE_INFO&&menu!=PAGE_INFO)info_parent=menu;if(menu>=0&&menu<16)saved_selection[menu]=selected;menu=page;selected=page>=0&&page<16?saved_selection[page]:0;if(selected>=page_count())selected=0;if(page==PAGE_CONFIRM)selected=0;menu_anim_start=0;if(page!=PAGE_PLAYER)media_release_art();dirty=1;}
static void page_back(void){switch(menu){case PAGE_CLOCK:break;case PAGE_APPS:page_open(PAGE_CLOCK);break;case PAGE_TIMER_CUSTOM:page_open(PAGE_TIMER);break;case PAGE_ALARM_CUSTOM:page_open(PAGE_ALARM);break;case PAGE_MODES:case PAGE_POWER:page_open(PAGE_SETTINGS);break;case PAGE_CONFIRM:page_open(PAGE_POWER);break;case PAGE_PLAYER:page_open(PAGE_MEDIA);break;case PAGE_MEDIA_MORE:page_open(PAGE_PLAYER);break;case PAGE_EDIT:page_open(edit_parent);break;case PAGE_INFO:page_open(info_parent);break;default:page_open(PAGE_APPS);break;}}
static void editor(int field,const char *label,int value,int lo,int hi,int step){edit_parent=menu;edit_field=field;edit_label=label;edit_value=value;edit_min=lo;edit_max=hi;edit_step=step;page_open(PAGE_EDIT);}
static void settings_commit(Settings old){if(memcmp(&old,&cfg,sizeof cfg)&&save())cfg=old;brightness();dirty=1;}
static void watch_toggle(void){if(watch_start){watch_elapsed+=mono()-watch_start;watch_start=0;}else watch_start=mono();}
static void choose(void){Settings old=cfg;switch(menu){
 case PAGE_APPS:{static const int pages[]={PAGE_CLOCK,PAGE_TIMER,PAGE_WATCH,PAGE_ALARM,PAGE_MEDIA,PAGE_SETTINGS};page_open(pages[selected]);break;}
 case PAGE_TIMER:if(selected<3){static const int presets[]={300,900,1500};timer_end=mono()+presets[selected];alert=0;page_open(PAGE_CLOCK);}else if(selected==3)page_open(PAGE_TIMER_CUSTOM);else if(selected==4){timer_end=0;alert=0;}else page_back();break;
 case PAGE_TIMER_CUSTOM:if(selected<3)editor(selected,selected==0?"Hours":selected==1?"Minutes":"Seconds",custom_timer[selected],0,selected==0?23:59,1);else{int seconds=custom_timer[0]*3600+custom_timer[1]*60+custom_timer[2];if(seconds){timer_end=mono()+seconds;alert=0;page_open(PAGE_CLOCK);}}break;
 case PAGE_WATCH:if(selected==0)watch_toggle();else if(selected==1)watch_lap=watch_elapsed+(watch_start?mono()-watch_start:0);else if(selected==2){watch_elapsed=watch_start=watch_lap=0;}else page_back();break;
 case PAGE_ALARM:if(selected<2){cfg.alarm_hour=selected?8:7;cfg.alarm_minute=0;cfg.last_alarm_day=-1;page_open(PAGE_CLOCK);}else if(selected==2){custom_alarm[0]=cfg.alarm_hour>=0?cfg.alarm_hour:7;custom_alarm[1]=cfg.alarm_minute;page_open(PAGE_ALARM_CUSTOM);}else if(selected==3)cfg.alarm_hour=-1;else page_back();break;
 case PAGE_ALARM_CUSTOM:if(selected<2)editor(selected,selected?"Minutes":"Hour (24h)",custom_alarm[selected],0,selected?59:23,1);else{cfg.alarm_hour=custom_alarm[0];cfg.alarm_minute=custom_alarm[1];cfg.last_alarm_day=-1;page_open(PAGE_CLOCK);}break;
 case PAGE_SETTINGS:if(selected==0)page_open(PAGE_MODES);else if(selected==1)editor(0,"Brightness",cfg.brightness,1,100,1);else if(selected==2)editor(1,"UTC offset (minutes)",cfg.offset,-720,840,15);else if(selected==3){static const char *names[]={"midnight","teal","amber","blue"};int n;for(n=0;n<4;n++)if(!strcmp(cfg.theme_name,names[n]))break;strcpy(cfg.theme_name,names[(n+1)%4]);theme_load();}else if(selected==4)cfg.hold_indicator=!cfg.hold_indicator;else page_open(PAGE_POWER);break;
 case PAGE_MODES:cfg.profile=selected;page_open(PAGE_CLOCK);break;
 case PAGE_POWER:pending_power=selected+1;page_open(PAGE_CONFIRM);break;
 case PAGE_CONFIRM:if(selected==1)power_action(pending_power);else page_back();break;
 case PAGE_MEDIA:page_open(PAGE_PLAYER);break;
 case PAGE_PLAYER:if(selected==0&&media_can_prev)media_send("previous",0);else if(selected==1&&media_can_play)media_send(media_playing?"pause":"play",0);else if(selected==2&&media_can_next)media_send("next",0);else if(selected==3){if(media_live()&&media_volume_ok)editor(0,"Windows volume",media_volume<0?50:media_volume,0,100,5);else{info_text="Volume unavailable on this player";page_open(PAGE_INFO);}}else if(selected==4)page_open(PAGE_MEDIA_MORE);break;
 case PAGE_MEDIA_MORE:if(selected==0){info_text="Lyrics are not shared by Windows";page_open(PAGE_INFO);}else if(selected==1){info_text="Audio plays on Windows / not TomTom";page_open(PAGE_INFO);}else page_back();break;
 case PAGE_EDIT:if(edit_parent==PAGE_TIMER_CUSTOM)custom_timer[edit_field]=edit_value;else if(edit_parent==PAGE_ALARM_CUSTOM)custom_alarm[edit_field]=edit_value;else if(edit_parent==PAGE_SETTINGS){if(edit_field==0)cfg.brightness=edit_value;else cfg.offset=edit_value;}else if(edit_parent==PAGE_PLAYER)media_send("volume",edit_value);page_open(edit_parent);break;
 case PAGE_INFO:page_back();break;
 default:page_open(PAGE_APPS);break;}
 settings_commit(old);}
static void event(const char *kind){int before=selected;dirty=1;if(alert){alert=0;return;}if(!strcmp(kind,"home")){page_open(PAGE_CLOCK);return;}if(!strcmp(kind,"hold")||!strcmp(kind,"long")||!strcmp(kind,"back")){page_back();return;}
 if(menu==PAGE_EDIT&&(!strcmp(kind,"tap")||!strcmp(kind,"double"))){edit_value+=!strcmp(kind,"tap")?edit_step:-edit_step;if(edit_value>edit_max)edit_value=edit_min;if(edit_value<edit_min)edit_value=edit_max;return;}
 if(!strcmp(kind,"power")||!strcmp(kind,"double")){if(menu)choose();else page_open(PAGE_APPS);return;}
 if(!strcmp(kind,"tap")){if(menu){selected=(selected+1)%page_count();if(menu!=PAGE_PLAYER&&menu!=PAGE_EDIT){menu_anim_from=69+(before%4)*32;menu_anim_to=69+(selected%4)*32;menu_anim_start=(before/4==selected/4)?mono():0;}}else page_open(PAGE_APPS);}}
static void page_label(int i,char *s,size_t cap,int *symbol){const char *v="Back";*symbol=9;switch(menu){
 case PAGE_APPS:{static const char *labels[]={"Clock","Timer","Stopwatch","Alarm","Media","Settings"};static const int icons[]={0,3,5,8,11,12};v=labels[i];*symbol=icons[i];break;}
 case PAGE_TIMER:{static const char *labels[]={"5 minutes","15 minutes","25 minutes","Custom duration","Cancel timer","Back"};v=labels[i];*symbol=3;break;}
 case PAGE_TIMER_CUSTOM:if(i<3){snprintf(s,cap,"%s   %02d",i==0?"Hours":i==1?"Minutes":"Seconds",custom_timer[i]);*symbol=3;return;}v="Start timer";*symbol=3;break;
 case PAGE_WATCH:v=i==0?(watch_start?"Pause":watch_elapsed?"Resume":"Start"):i==1?"Record lap":i==2?"Reset":"Back";*symbol=5;break;
 case PAGE_ALARM:{static const char *labels[]={"07:00 daily","08:00 daily","Custom time","Disable alarm","Back"};v=labels[i];*symbol=8;break;}
 case PAGE_ALARM_CUSTOM:if(i<2){snprintf(s,cap,"%s   %02d",i?"Minutes":"Hour",custom_alarm[i]);*symbol=8;return;}v="Save daily alarm";*symbol=8;break;
 case PAGE_SETTINGS:{static const char *labels[]={"Clock mode","Brightness","Time zone","Theme","Hold indicator","Power"};static const int icons[]={0,6,7,12,6,13};v=labels[i];*symbol=icons[i];if(i==3){snprintf(s,cap,"Theme / %s",cfg.theme_name);return;}if(i==4){snprintf(s,cap,"Hold indicator / %s",cfg.hold_indicator?"on":"off");return;}break;}
 case PAGE_MODES:{static const char *labels[]={"Minimal","Desk","Bedside"};v=labels[i];*symbol=i;break;}
 case PAGE_POWER:v=i==0?"Suspend":i==1?"Restart":"Shut down";*symbol=13;break;
 case PAGE_CONFIRM:v=i==0?"Cancel":pending_power==1?"Suspend now":pending_power==2?"Restart now":"Shut down now";*symbol=i?13:9;break;
 case PAGE_MEDIA:v="Spotify";*symbol=11;break;
 case PAGE_MEDIA_MORE:v=i==0?"Lyrics / subtitles":i==1?"Audio output":"Back";*symbol=11;break;
 default:break;}snprintf(s,cap,"%s",v);}
static void navigation_draw(void){char s[96],a[24],b[24];int i,start=(selected/4)*4;uint16_t fg=theme_color(theme.foreground),muted=theme_color(theme.muted),accent=theme_color(theme.accent),bg=theme_color(theme.background);double now=mono();
 if(menu==PAGE_PLAYER){media_draw();return;}text_fit(18,8,page_title(),270,3,fg);
 if(menu==PAGE_EDIT){snprintf(s,sizeof s,"%d",edit_value);center(85,s,5,accent);center(160,"Tap + / double tap -",1,muted);center(191,"Power: apply / hold 1.5s: cancel",1,muted);return;}
 if(menu==PAGE_INFO){text_fit(16,90,info_text,288,1,fg);center(185,"Power to go back",1,muted);return;}
 if(menu==PAGE_WATCH){duration_text(a,sizeof a,(int)(watch_elapsed+(watch_start?now-watch_start:0)));duration_text(b,sizeof b,(int)watch_lap);snprintf(s,sizeof s,"%s   /   Lap %s",a,b);text(20,43,s,1,accent);}
 else if(menu==PAGE_CONFIRM)text(20,43,pending_power==1?"Alarms cannot wake the device":"Clock will stop until restarted",1,muted);
 else if(menu==PAGE_ALARM)text(20,43,"Daily / silent visual alerts",1,muted);
 else if(menu==PAGE_MEDIA)text(20,43,"Controls your connected Spotify player",1,muted);
 else if(page_count()>4){snprintf(s,sizeof s,"%d / %d",selected+1,page_count());text(255,43,s,1,muted);}
 {double progress=menu_anim_start?(now-menu_anim_start)/0.18:1;int y;if(progress>1)progress=1;if(progress<0)progress=0;progress=1-(1-progress)*(1-progress);y=menu_anim_start?(int)(menu_anim_from+(menu_anim_to-menu_anim_from)*progress):69+(selected%4)*32;rounded(12,y,296,30,11,theme_color(theme.surface));}
 for(i=start;i<start+4&&i<page_count();i++){int y=73+(i-start)*32,symbol;uint16_t surface=i==selected?theme_color(theme.surface):bg;page_label(i,s,sizeof s,&symbol);icon(23,y-2,symbol,accent,surface);text_fit(57,y-2,s,238,2,i==selected?fg:muted);}
 center(221,"Tap: browse / power: select",1,muted);}
