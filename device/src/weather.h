/* Weather is a bounded summary supplied by the hub. No HTTP/parser/assets here. */
static int weather_code,weather_temp,weather_unusual;
static double weather_at;
static int weather_live(void){return cfg.weather&&weather_at>0&&mono()-weather_at<1800;}
static int weather_visible(void){return weather_live()&&(cfg.weather==2||weather_unusual);}
static const char *weather_label(int code){if(code>=95)return "Thunderstorm";if(code==66||code==67)return "Freezing rain";if(code>=71&&code<=77)return "Snow";if(code==85||code==86)return "Snow showers";if(code>=80)return "Rain showers";if(code>=61)return "Rain";if(code>=51)return "Drizzle";if(code>=45)return "Fog";if(code>=2)return "Cloudy";return "Clear";}
static void weather_icon(int x,int y,int code,uint16_t c,uint16_t bg){int i;if(code<=1){icon(x,y,6,c,bg);return;}if(code==45||code==48){for(i=0;i<3;i++)rounded(x+i%2*3,y+3+i*6,22-i%2*6,2,1,c);return;}
 rounded(x+4,y+3,12,12,6,c);rounded(x+12,y+7,10,8,4,c);rounded(x,y+8,23,8,4,c);rounded(x+3,y+10,17,4,2,bg);
 if(code>=95){stroke(x+14,y+13,x+9,y+18,c);stroke(x+9,y+18,x+14,y+18,c);stroke(x+14,y+18,x+10,y+23,c);}else if((code>=71&&code<=77)||code==85||code==86){for(i=0;i<3;i++){rect(x+3+i*7,y+18,2,5,c);rect(x+2+i*7,y+19,4,2,c);}}else if(code>=51){for(i=0;i<3;i++)stroke(x+5+i*7,y+18,x+3+i*7,y+22,c);}}
static int weather_command(const char *name,const char *a,const char *b){int code,temp,unusual;char extra;
 if(!strcmp(name,"weather-clear")&&!a){weather_at=0;return 1;}
 if(strcmp(name,"weather-state")||!cfg.weather||!a||b)return 0;
 if(sscanf(a,"%d,%d,%d%c",&code,&temp,&unusual,&extra)!=3||code<0||code>99||temp< -1000||temp>700||unusual<0||unusual>1)return 0;
 weather_code=code;weather_temp=temp;weather_unusual=unusual;weather_at=mono();return 1;}
