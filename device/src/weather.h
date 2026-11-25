/* Weather is a bounded summary supplied by the hub. No HTTP/parser/assets here. */
static int weather_code,weather_temp,weather_unusual;
static double weather_at;
static int weather_live(void){return cfg.weather&&weather_at>0&&mono()-weather_at<1800;}
static int weather_visible(void){return weather_live()&&(cfg.weather==2||weather_unusual);}
static const char *weather_label(int code){switch(code){
 case 0:return "Clear";case 1:return "Mostly clear";case 2:return "Partly cloudy";case 3:return "Overcast";
 case 45:return "Fog";case 48:return "Rime fog";
 case 51:return "Light drizzle";case 53:return "Drizzle";case 55:return "Heavy drizzle";
 case 56:return "Light freezing drizzle";case 57:return "Heavy freezing drizzle";
 case 61:return "Light rain";case 63:return "Rain";case 65:return "Heavy rain";
 case 66:return "Light freezing rain";case 67:return "Heavy freezing rain";
 case 71:return "Light snow";case 73:return "Snow";case 75:return "Heavy snow";case 77:return "Snow grains";
 case 80:return "Light rain showers";case 81:return "Rain showers";case 82:return "Heavy rain showers";
 case 85:return "Snow showers";case 86:return "Heavy snow showers";
 case 95:return "Thunderstorm";case 96:return "Thunderstorm with hail";case 99:return "Thunderstorm with heavy hail";
 default:return "Unknown";}}
static void weather_icon(int x,int y,int code,uint16_t c,uint16_t bg){int i,snow=(code==71||code==73||code==75||code==77||code==85||code==86),storm=(code==95||code==96||code==99),fog=(code==45||code==48),wet=(code>=51&&code<=67)||(code>=80&&code<=82);if(code<=1){icon(x,y,6,c,bg);return;}
 if(code==2){icon(x+1,y,6,c,bg);rounded(x+9,y+8,12,8,4,c);rounded(x+2,y+9,21,8,4,c);rounded(x+5,y+11,15,4,2,bg);return;}
 if(fog){for(i=0;i<3;i++)rounded(x+1+(i%2)*2,y+3+i*6,22-(i%2)*3,2,1,c);return;}
 rounded(x+4,y+3,12,12,6,c);rounded(x+12,y+7,10,8,4,c);rounded(x,y+8,23,8,4,c);rounded(x+3,y+10,17,4,2,bg);
 if(storm){stroke(x+14,y+13,x+9,y+18,c);stroke(x+9,y+18,x+14,y+18,c);stroke(x+14,y+18,x+10,y+23,c);if(code>95)for(i=0;i<2;i++)rect(x+3+i*7,y+19+i%2*3,2,2,c);}
 else if(snow){for(i=0;i<3;i++){stroke(x+4+i*7,y+18,x+4+i*7,y+23,c);stroke(x+2+i*7,y+20,x+6+i*7,y+20,c);}}
 else if(wet){for(i=0;i<3;i++)stroke(x+5+i*7,y+18,x+3+i*7,y+22,c);}}
static int weather_command(const char *name,const char *a,const char *b){int code,temp,unusual;char extra;
 if(!strcmp(name,"weather-clear")&&!a){weather_at=0;return 1;}
 if(strcmp(name,"weather-state")||!cfg.weather||!a||b)return 0;
 if(sscanf(a,"%d,%d,%d%c",&code,&temp,&unusual,&extra)!=3||code<0||code>99||temp< -1000||temp>700||unusual<0||unusual>1)return 0;
 weather_code=code;weather_temp=temp;weather_unusual=unusual;weather_at=mono();return 1;}
