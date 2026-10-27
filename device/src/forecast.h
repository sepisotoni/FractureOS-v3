/* Forecast text is parsed only inside Weather. The SD cache survives hub loss. */
typedef struct {long when;int temp,code,rain;} ForecastHour;
typedef struct {long when;int low,high,code,rain;} ForecastDay;
typedef struct {long saved;int offset,nhours,ndays;ForecastHour hour[24];ForecastDay day[7];} Forecast;
static Forecast *forecast;
static long forecast_saved;
static void forecast_path(char *out,size_t cap){const char *slash=strrchr(config_path,'/');snprintf(out,cap,"%.*s/weather.dat",slash?(int)(slash-config_path):1,slash?config_path:".");}
static void forecast_header(void){char path[640],magic[8];int offset;long saved;FILE *f;forecast_path(path,sizeof path);f=fopen(path,"r");forecast_saved=0;if(!f)return;if(fscanf(f,"%7s %ld %d",magic,&saved,&offset)==3&&!strcmp(magic,"FWE1")&&saved>=1700000000&&offset>=-43200&&offset<=50400)forecast_saved=saved;fclose(f);}
static void forecast_release(void){free(forecast);forecast=NULL;}
static void forecast_load(void){char path[640],line[160],magic[8],extra,kind;FILE *f;Forecast *next;long when;int a,b,c,d;forecast_path(path,sizeof path);f=fopen(path,"r");if(!f)return;next=calloc(1,sizeof *next);if(!next){fclose(f);return;}
 if(!fgets(line,sizeof line,f)||sscanf(line,"%7s %ld %d %c",magic,&next->saved,&next->offset,&extra)!=3||strcmp(magic,"FWE1")||next->saved<1700000000||next->offset< -43200||next->offset>50400){free(next);fclose(f);return;}
 while(fgets(line,sizeof line,f)){if(line[0]=='H'&&next->nhours<24&&sscanf(line,"%c %ld %d %d %d %c",&kind,&when,&a,&b,&c,&extra)==5&&when>=1700000000&&a>=-1000&&a<=700&&b>=0&&b<=99&&c>=-1&&c<=100){ForecastHour *h=&next->hour[next->nhours++];h->when=when;h->temp=a;h->code=b;h->rain=c;}
 else if(line[0]=='D'&&next->ndays<7&&sscanf(line,"%c %ld %d %d %d %d %c",&kind,&when,&a,&b,&c,&d,&extra)==6&&when>=1700000000&&a>=-1000&&b<=700&&a<=b&&c>=0&&c<=99&&d>=-1&&d<=100){ForecastDay *day=&next->day[next->ndays++];day->when=when;day->low=a;day->high=b;day->code=c;day->rain=d;}else{free(next);fclose(f);return;}}
 fclose(f);if(!next->nhours||!next->ndays){free(next);return;}forecast_release();forecast=next;forecast_saved=next->saved;}
static void forecast_age(char *out,size_t cap){long age=(long)time(NULL)-forecast_saved;if(!forecast_saved)snprintf(out,cap,"Waiting for stored forecast");else if(age<0)snprintf(out,cap,"Cached forecast / check clock");else if(age<3600)snprintf(out,cap,"Cached %ld min ago",age/60);else if(age<86400)snprintf(out,cap,"Cached %ld hours ago",age/3600);else snprintf(out,cap,"Cached %ld days ago",age/86400);}
static void forecast_time(char *out,size_t cap,long when,const char *format){time_t value=when+(forecast?forecast->offset:cfg.offset*60);struct tm *t=gmtime(&value);if(t)strftime(out,cap,format,t);else out[0]=0;}
